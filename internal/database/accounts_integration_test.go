//go:build integration

package database

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAccountsMigrationPreservesIdentity(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := pgx.Identifier{fmt.Sprintf("accounts_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `CREATE TABLE schema_migrations (filename TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	names, err := listMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name >= "0008_accounts.sql" {
			break
		}
		if err := applyOne(ctx, db, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `
 INSERT INTO hosts (name,email,password_hash,google_sub) VALUES ('Existing','existing@example.com','hash','google-id');
 INSERT INTO events (host_id,name) VALUES (1,'Existing event');
 INSERT INTO event_members (event_id,host_id,display,role) VALUES (1,1,'Existing','host');
 INSERT INTO sessions (token,host_id,expires_at) VALUES ('existing-token',1,NOW()+INTERVAL '1 day');
 `); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	var count int
	err = db.QueryRow(ctx, `SELECT count(*) FROM accounts a
 JOIN events e ON e.account_id=a.id
 JOIN event_members m ON m.account_id=a.id AND m.event_id=e.id
 JOIN sessions s ON s.account_id=a.id
 WHERE a.password_hash='hash' AND a.google_sub='google-id' AND m.role='host' AND s.token='existing-token'`).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("preserved identity count=%d, error=%v", count, err)
	}
	var id int64
	if err := db.QueryRow(ctx, `INSERT INTO accounts (name,email) VALUES ('New','new@example.com') RETURNING id`).Scan(&id); err != nil || id != 2 {
		t.Fatalf("sequence id=%d, error=%v", id, err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions (token,account_id,expires_at) VALUES ('invalid',999,NOW())`); err == nil {
		t.Fatal("expected foreign key rejection")
	}
}
