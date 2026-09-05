terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = "project-4ddffd8b-3b42-486b-b6a"
  region  = "asia-east1"
}

# ------------------------------------------------------------------------------
# 1. Enable Required GCP APIs
# ------------------------------------------------------------------------------
resource "google_project_service" "run_api" {
  service            = "run.googleapis.com"
  disable_on_destroy = false
}

# ------------------------------------------------------------------------------
# 2. Service Account for Cloud Run Runtime
# ------------------------------------------------------------------------------
resource "google_service_account" "go_backend_sa" {
  account_id   = "go-backend-runner"
  display_name = "Cloud Run Service Account for Go Backend"
}

# ------------------------------------------------------------------------------
# 3. Cloud Run Service Deployment (v2 API)
# ------------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "go_backend" {
  name     = "go-backend-api"
  location = "us-central1"
  ingress  = "INGRESS_TRAFFIC_ALL" # Open to public internet at Google's edge

  template {
    service_account = google_service_account.go_backend_sa.email

    containers {
      # Public GCP sample app to verify deployment; swap with specified Artifact Registry URL later
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

  depends_on = [google_project_service.run_api]
}

# ------------------------------------------------------------------------------
# 4. IAM Access Control (Public Endpoints)
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
# 5. Outputs
# ------------------------------------------------------------------------------
output "cloud_run_url" {
  description = "The public URL of the deployed Cloud Run service"
  value       = google_cloud_run_v2_service.go_backend.uri
}