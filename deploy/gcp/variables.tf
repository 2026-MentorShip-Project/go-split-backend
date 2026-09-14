variable "gcp_project_id" {
  type        = string
  description = "GCP Project ID"
}

variable "gcp_region" {
  type        = string
  description = "GCP Region"
  default     = "us-central1"
}

variable "cloud_run_service" {
  type        = string
  description = "The name of the Cloud Run service"
}

variable "gar_repository_id" {
  type        = string
  description = "The ID of the Artifact Registry repository"
}

variable "db_password" {
  type        = string
  description = "Password for the application PostgreSQL user"
  sensitive   = true
}

variable "google_client_id" {
  type        = string
  description = "OAuth client ID that Google ID tokens must be issued for; empty disables Google sign-in"
  default     = ""
  sensitive   = true
}
