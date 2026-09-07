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
# Service Account for Cloud Run Runtime
# ------------------------------------------------------------------------------
resource "google_service_account" "go_backend_sa" {
  account_id   = "go-backend-runner"
  display_name = "Cloud Run Service Account for Go Backend"
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

  depends_on = [google_project_service.run_api]
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
