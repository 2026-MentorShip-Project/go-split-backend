#!/usr/bin/env bash
set -euo pipefail

# 1. Define Variables (Replace with your values)
export PROJECT_ID="project-4ddffd8b-3b42-486b-b6a"
export REGION="asia-east1"
export GCS_BUCKET_NAME="${PROJECT_ID}-tfstate"
export GITHUB_REPO="2026-MentorShip-Project/go-split-backend"
# Billing account for the budget alert (gcloud billing accounts list). Leave
# empty to skip; Terraform then skips the budget too.
export BILLING_ACCOUNT_ID=""

gcloud config set project "${PROJECT_ID}"

# 2. Enable Required GCP APIs
gcloud services enable \
  storage.googleapis.com \
  iam.googleapis.com \
  iamcredentials.googleapis.com \
  cloudresourcemanager.googleapis.com \
  sts.googleapis.com \
  artifactregistry.googleapis.com \
  sqladmin.googleapis.com \
  secretmanager.googleapis.com

# 3. Create GCS Bucket for Terraform Remote State
gcloud storage buckets create "gs://${GCS_BUCKET_NAME}" \
  --location="${REGION}" \
  --uniform-bucket-level-access
gcloud storage buckets update "gs://${GCS_BUCKET_NAME}" --versioning

# 4. Create Service Account for GitHub Actions / CD Runner
gcloud iam service-accounts create github-cd-runner \
  --display-name="GitHub Actions CD Runner"

# 5. Grant Required IAM Roles to the CD Runner Service Account
# (Add extra roles here as needed, e.g., roles/cloudsql.admin)
for ROLE in "roles/run.admin" "roles/artifactregistry.admin" "roles/cloudsql.admin" "roles/secretmanager.admin" "roles/resourcemanager.projectIamAdmin" "roles/iam.serviceAccountAdmin" "roles/iam.serviceAccountUser" "roles/serviceusage.serviceUsageAdmin" "roles/storage.admin"; do
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member="serviceAccount:github-cd-runner@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="${ROLE}"
done

gcloud storage buckets add-iam-policy-binding "gs://${GCS_BUCKET_NAME}" \
  --member="serviceAccount:github-cd-runner@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/storage.objectAdmin"

# Let Terraform manage the budget (deploy/gcp/billing.tf). The role is granted on
# the billing account, not the project, so whoever runs this needs Billing
# Account Administrator. On an existing setup, run just this command, then set
# the BILLING_ACCOUNT_ID variable in GitHub.
if [ -n "${BILLING_ACCOUNT_ID}" ]; then
  gcloud billing accounts add-iam-policy-binding "${BILLING_ACCOUNT_ID}" \
    --member="serviceAccount:github-cd-runner@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="roles/billing.costsManager"
fi

# 6. Create Workload Identity Pool
gcloud iam workload-identity-pools create "github-pool" \
  --location="global" \
  --display-name="GitHub Actions Pool"

# 7. Create Workload Identity Provider for GitHub OIDC
gcloud iam workload-identity-pools providers create-oidc "github-provider" \
  --location="global" \
  --workload-identity-pool="github-pool" \
  --display-name="GitHub Provider" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
  --attribute-condition="assertion.repository == '${GITHUB_REPO}'" \
  --issuer-uri="https://token.actions.githubusercontent.com"

# 8. Allow GitHub Actions to Impersonate the Service Account
PROJECT_NUMBER=$(gcloud projects describe "${PROJECT_ID}" --format="value(projectNumber)")

gcloud iam service-accounts add-iam-policy-binding "github-cd-runner@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-pool/attribute.repository/${GITHUB_REPO}"

# 9. Output your WIF Provider String (Save this for your GitHub workflow!)
echo "Your WIF Provider Value:"
echo "projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-pool/providers/github-provider"
