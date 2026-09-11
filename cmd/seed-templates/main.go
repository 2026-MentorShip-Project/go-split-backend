// Package main runs the template seeder as a one-shot command.
//
// Usage:
//
//	DB_HOST=... DB_PORT=... DB_USER=... DB_PASSWORD=... DB_NAME=... \
//	    go run ./cmd/seed-templates
//
// The command opens the same pool the API uses, runs any pending
// migrations (so the templates table exists even on a fresh database),
// then upserts every embedded template. It is idempotent and safe to
// re-run on every deploy.
package main

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	"go-split-backend/internal/database"
	"go-split-backend/internal/events"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.Open(ctx)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	labels, err := events.SeedTemplates(ctx, db)
	if err != nil {
		log.Fatalf("seed templates: %v", err)
	}
	log.WithField("templates", labels).Info("templates seeded")
}
