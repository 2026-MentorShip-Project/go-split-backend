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

variable "pgo_bucket_name" {
  type        = string
  description = "Globally unique GCS bucket name for CPU profiles"
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

variable "allowed_origins" {
  type        = string
  description = "Comma-separated browser origins allowed for credentialed CORS (scheme + host + port, no path)"
  default     = "https://go-split.vercel.app,http://localhost:3000"
}

variable "vertex_location" {
  type        = string
  description = "Vertex AI location for rule drafting, e.g. global (asia-east1 serves no Gemini text models); empty disables the feature"
  default     = ""
}

variable "vertex_model" {
  type        = string
  description = "Vertex AI model id used to draft split rules, e.g. gemini-3.5-flash-lite; empty disables the feature"
  default     = ""
}

variable "cloud_run_max_instances" {
  type        = number
  description = "Cloud Run instance cap. Each instance opens at least 4 database connections (pgx default), so instances x 4 must stay under the Cloud SQL limit (about 25 on db-f1-micro, a few reserved)."
  default     = 5
}

variable "db_disk_autoresize_limit_gb" {
  type        = number
  description = "Largest size, in GB, that Cloud SQL autoresize may grow the disk to"
  default     = 20
}

variable "gar_keep_images" {
  type        = number
  description = "Most recent backend images Artifact Registry keeps regardless of age"
  default     = 10
}

variable "billing_account_id" {
  type        = string
  description = "Billing account ID (XXXXXX-XXXXXX-XXXXXX) for the budget alert; empty skips the budget"
  default     = ""
}

variable "monthly_budget" {
  type        = number
  description = "Monthly budget, in whole units of the billing account's currency; alerts at 50%, 90% and 100%, plus a forecast"
  default     = 20
}

variable "budget_alert_emails" {
  type        = list(string)
  description = "Extra addresses for budget alerts, beyond the billing account's admins and users"
  default     = []
}
