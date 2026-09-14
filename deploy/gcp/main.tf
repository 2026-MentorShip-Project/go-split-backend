terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }

  backend "gcs" {
    prefix = "terraform/state"
  }
}

provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
}

# ------------------------------------------------------------------------------
# Enable Required GCP APIs
# ------------------------------------------------------------------------------
resource "google_project_service" "run_api" {
  service            = "run.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "artifact_registry_api" {
  service            = "artifactregistry.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "sqladmin_api" {
  service            = "sqladmin.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "secretmanager_api" {
  service            = "secretmanager.googleapis.com"
  disable_on_destroy = false
}

# ------------------------------------------------------------------------------
# Artifact Registry
# ------------------------------------------------------------------------------
resource "google_artifact_registry_repository" "backend" {
  location      = var.gcp_region
  repository_id = var.gar_repository_id
  format        = "DOCKER"

  depends_on = [google_project_service.artifact_registry_api]
}

# ------------------------------------------------------------------------------
# Cloud SQL for PostgreSQL
# ------------------------------------------------------------------------------
resource "google_sql_database_instance" "postgres" {
  name                = "go-split-postgres"
  database_version    = "POSTGRES_15"
  region              = var.gcp_region
  deletion_protection = false

  settings {
    tier              = "db-f1-micro"
    availability_type = "ZONAL"
    disk_type         = "PD_SSD"
    disk_size         = 10
    disk_autoresize   = true

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }

    ip_configuration {
      ipv4_enabled = true
      ssl_mode     = "ENCRYPTED_ONLY"
    }
  }

  depends_on = [google_project_service.sqladmin_api]
}

resource "google_sql_database" "app" {
  name     = "go_split"
  instance = google_sql_database_instance.postgres.name
}

resource "google_sql_user" "app" {
  name     = "go_split"
  instance = google_sql_database_instance.postgres.name
  password = var.db_password
}

resource "google_secret_manager_secret" "db_password" {
  secret_id = "go-split-db-password"

  replication {
    auto {}
  }

  depends_on = [google_project_service.secretmanager_api]
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password
}

# ------------------------------------------------------------------------------
# Service Account for Cloud Run Runtime
# ------------------------------------------------------------------------------
resource "google_service_account" "go_backend_sa" {
  account_id   = "go-backend-runner"
  display_name = "Cloud Run Service Account for Go Backend"
}

resource "google_project_iam_member" "cloud_run_sql_client" {
  project = var.gcp_project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.go_backend_sa.email}"
}

resource "google_secret_manager_secret_iam_member" "cloud_run_db_password_accessor" {
  secret_id = google_secret_manager_secret.db_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.go_backend_sa.email}"
}


# ------------------------------------------------------------------------------
# Cloud Run Service Deployment (v2 API)
# ------------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "go_backend" {
  name     = var.cloud_run_service
  location = var.gcp_region
  ingress  = "INGRESS_TRAFFIC_ALL" # Open to public internet at Google's edge

  template {
    service_account = google_service_account.go_backend_sa.email

    volumes {
      name = "cloudsql"

      cloud_sql_instance {
        instances = [google_sql_database_instance.postgres.connection_name]
      }
    }

    containers {
      # Dummy/Hello world image for initial setup
      image = "us-docker.pkg.dev/cloudrun/container/hello:latest"

      ports {
        container_port = 8080 # Cloud Run expects HTTP on 8080 by default
      }

      resources {
        limits = {
          cpu    = "1000m" # 1 vCPU
          memory = "512Mi" # 512 MB RAM
        }
      }

      env {
        name  = "APP_ENV"
        value = "production"
      }

      env {
        name  = "GOOGLE_CLIENT_ID"
        value = var.google_client_id
      }

      env {
        name  = "DB_HOST"
        value = "/cloudsql/${google_sql_database_instance.postgres.connection_name}"
      }

      env {
        name  = "DB_PORT"
        value = "5432"
      }

      env {
        name  = "DB_USER"
        value = google_sql_user.app.name
      }

      env {
        name  = "DB_NAME"
        value = google_sql_database.app.name
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }

      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }
    }

    scaling {
      min_instance_count = 0 # Scales to zero to save costs when idle
      max_instance_count = 10
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      template[0].annotations["client.knative.dev/user-image"]
    ]
  }

  depends_on = [google_project_service.run_api, google_project_iam_member.cloud_run_sql_client, google_secret_manager_secret_iam_member.cloud_run_db_password_accessor]
}

# ------------------------------------------------------------------------------
# IAM Access Control (Public Endpoints)
# ------------------------------------------------------------------------------
# Allows unauthenticated callers on the internet to invoke the Cloud Run URL.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  project  = google_cloud_run_v2_service.go_backend.project
  location = google_cloud_run_v2_service.go_backend.location
  name     = google_cloud_run_v2_service.go_backend.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# ------------------------------------------------------------------------------
# Outputs
# ------------------------------------------------------------------------------
output "cloud_run_url" {
  description = "The public URL of the deployed Cloud Run service"
  value       = google_cloud_run_v2_service.go_backend.uri
}
