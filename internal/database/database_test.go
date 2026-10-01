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

func TestParseMaxConnsAcceptsPositiveInteger(t *testing.T) {
	got, err := parseMaxConns("4")
	if err != nil {
		t.Fatalf("parseMaxConns: %v", err)
	}
	if got != 4 {
		t.Fatalf("got %d, want 4", got)
	}
}

func TestParseMaxConnsRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"0", "-1", "four", "4.5", "99999999999"} {
		if _, err := parseMaxConns(raw); err == nil {
			t.Errorf("parseMaxConns(%q): expected an error", raw)
		}
	}
}

func TestOpenRejectsInvalidMaxConns(t *testing.T) {
	for name, value := range map[string]string{
		"DB_HOST": "127.0.0.1", "DB_USER": "u", "DB_PASSWORD": "p", "DB_NAME": "d", "DB_PORT": "5432",
		"DB_MAX_CONNS": "zero",
	} {
		t.Setenv(name, value)
	}

	if _, err := Open(context.Background()); err == nil {
		t.Fatal("expected an invalid DB_MAX_CONNS to fail")
	}
}
