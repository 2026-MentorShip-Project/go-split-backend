# ------------------------------------------------------------------------------
# Billing budget
# ------------------------------------------------------------------------------
# Emails when this project's spend crosses a share of the monthly budget. A
# budget only alerts; it never stops or caps spending.
#
# Skipped until var.billing_account_id is set. The CD service account also needs
# roles/billing.costsManager on the billing account (see deploy/boostrap.sh).

# The Budget API bills its calls to a quota project; without this override it
# falls back to Google's shared default project and the calls are refused.
provider "google" {
  alias                 = "billing"
  project               = var.gcp_project_id
  region                = var.gcp_region
  user_project_override = true
  billing_project       = var.gcp_project_id
}

data "google_project" "current" {}

resource "google_project_service" "billingbudgets_api" {
  service            = "billingbudgets.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "monitoring_api" {
  count              = var.billing_account_id != "" && length(var.budget_alert_emails) > 0 ? 1 : 0
  service            = "monitoring.googleapis.com"
  disable_on_destroy = false
}

# Extra recipients. Billing account admins and users get the emails regardless.
resource "google_monitoring_notification_channel" "budget_email" {
  for_each     = toset(var.billing_account_id == "" ? [] : var.budget_alert_emails)
  display_name = "Budget alert: ${each.value}"
  type         = "email"
  labels = {
    email_address = each.value
  }

  depends_on = [google_project_service.monitoring_api]
}

resource "google_billing_budget" "monthly" {
  count           = var.billing_account_id == "" ? 0 : 1
  provider        = google.billing
  billing_account = var.billing_account_id
  display_name    = "go-split monthly budget"

  budget_filter {
    projects = ["projects/${data.google_project.current.number}"]
  }

  amount {
    # No currency_code: the amount is in the billing account's own currency.
    specified_amount {
      units = tostring(var.monthly_budget)
    }
  }

  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.9
  }
  threshold_rules {
    threshold_percent = 1.0
  }
  # Warns before the month ends if the current rate would overshoot.
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }

  all_updates_rule {
    monitoring_notification_channels = [for c in google_monitoring_notification_channel.budget_email : c.id]
    disable_default_iam_recipients   = false
  }

  depends_on = [google_project_service.billingbudgets_api]
}
