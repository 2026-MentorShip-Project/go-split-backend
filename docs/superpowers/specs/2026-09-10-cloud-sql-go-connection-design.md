# Cloud SQL Go Connection Design

## Goal

Connect the Go API to the existing Cloud SQL PostgreSQL instance with the
smallest production-capable path.

## Decisions

- Use `pgxpool` for PostgreSQL connections.
- Use Cloud Run's Cloud SQL Unix socket at `/cloudsql/<connection-name>`.
- Read connection settings from environment variables.
- Fail startup if the database cannot be reached.
- Store the database password in Secret Manager and expose it to Cloud Run as
  a secret-backed environment variable.
- Provision the application database, user, Cloud SQL client permission, and
  Cloud SQL volume attachment in Terraform.

SQL migrations and sqlc-generated queries are separate follow-up work; neither
is needed to establish the connection.
