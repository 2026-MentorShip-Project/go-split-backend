# Cloud SQL Go Connection Implementation Plan

> **For the implementer:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to execute this plan task-by-task.

**Goal:** Make the Go API connect to PostgreSQL on startup in local and Cloud
Run environments.

**Architecture:** Add one small `internal/database` package using `pgxpool`.
Terraform creates the application database/user and grants the Cloud Run
service account access to Cloud SQL and the password secret. Cloud Run mounts
the instance's Unix socket and injects the remaining settings as environment
variables.

**Tech stack:** Go, pgx/v5, Terraform Google provider, Secret Manager.

### Task 1: Add the database connection package

- Add `pgx/v5`.
- Read `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME`.
- Parse the port and require the other values.
- Open a pool and ping it before returning.
- Add one focused test for required environment validation.

### Task 2: Connect the server at startup

- Load database configuration from the environment.
- Open the pool before serving HTTP.
- Close the pool during shutdown.
- Preserve the existing health endpoint and graceful shutdown behavior.

### Task 3: Wire Cloud SQL in Terraform

- Add the application database and user.
- Add the Secret Manager password secret and secret version input.
- Grant the runtime service account `roles/cloudsql.client` and secret access.
- Attach the Cloud SQL instance to Cloud Run and inject database settings.
- Replace deprecated `require_ssl` with the supported `ssl_mode` setting.

### Task 4: Verify and commit

- Run `gofmt`, `go test ./...`, and `terraform fmt`.
- Review the diff and commit with `Co-authored-by: Codex <codex@openai.com>`.
