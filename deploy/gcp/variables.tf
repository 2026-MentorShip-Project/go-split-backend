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