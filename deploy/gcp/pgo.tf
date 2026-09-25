resource "google_storage_bucket" "pgo_profiles" {
  name                        = var.pgo_bucket_name
  location                    = var.gcp_region
  uniform_bucket_level_access = true

  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }
}

resource "google_storage_bucket_iam_member" "pgo_profiles_writer" {
  bucket = google_storage_bucket.pgo_profiles.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${google_service_account.go_backend_sa.email}"
}
