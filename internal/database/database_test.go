package database

import (
	"context"
	"testing"
)

func TestOpenRequiresDatabaseSettings(t *testing.T) {
	for _, name := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_PORT"} {
		t.Setenv(name, "")
	}

	if _, err := Open(context.Background()); err == nil {
		t.Fatal("expected missing database settings to fail")
	}
}
