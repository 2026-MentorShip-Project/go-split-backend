//go:build integration

package events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"go-split-backend/internal/database"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEST_DATABASE_URL must point to a disposable database.
func TestCreateEventTemplateTransaction(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database")
	}
	ctx := t.Context()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedTemplates(ctx, db); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := db.QueryRow(ctx, `INSERT INTO accounts (name,email) VALUES ('test', $1) RETURNING id`, fmt.Sprintf("template-%d@example.com", time.Now().UnixNano())).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	content, err := loadTemplateContent(ctx, db, "烤肉/露營模板")
	if err != nil {
		t.Fatal(err)
	}
	create := func(template string) createEventResponse {
		t.Helper()
		event, err := createEventTx(ctx, db, accountID, createEventRequest{Name: "test", Template: template})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	event := create("烤肉/露營模板")
	for table, want := range map[string][]string{"event_item_tags": content.ItemTags, "event_cond_tags": content.CondTags} {
		got, err := loadTagLabels(ctx, db, event.ID, table)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v, want %v, err %v", table, got, want, err)
		}
		_, err = db.Exec(ctx, "INSERT INTO "+table+" (event_id,label,ordinal) VALUES ($1,$2,99)", event.ID, want[0])
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("duplicate %s: %v", table, err)
		}
	}
	rules, err := loadRules(ctx, db, event.ID)
	if err != nil || len(rules) != len(content.Rules) {
		t.Fatalf("rules: %v, %v", rules, err)
	}
	for i, rule := range rules {
		if rule.ItemTag != content.Rules[i].Tag {
			t.Fatalf("rule %d tag mismatch", i)
		}
	}
	// The same labels can be used by another event.
	create("烤肉/露營模板")
	custom := create("自訂")
	for _, table := range []string{"event_item_tags", "event_cond_tags", "event_rules"} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE event_id=$1", custom.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("custom %s: %d, %v", table, count, err)
		}
	}
	if _, err := createEventTx(ctx, db, accountID, createEventRequest{Name: "missing", Template: "聚餐模板"}); err != nil {
		t.Fatalf("missing template: %v", err)
	}
	defer func() {
		if _, err := SeedTemplates(context.Background(), db); err != nil {
			t.Error(err)
		}
	}()
	// A duplicate label causes settings insertion to fail after event insertion.
	if _, err := db.Exec(ctx, `UPDATE templates SET content=jsonb_set(content,'{item_tags}','["duplicate","duplicate"]') WHERE label='烤肉/露營模板'`); err != nil {
		t.Fatal(err)
	}
	if _, err := createEventTx(ctx, db, accountID, createEventRequest{Name: "rollback", Template: "烤肉/露營模板"}); err == nil {
		t.Fatal("expected duplicate label failure")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM events WHERE name = 'rollback'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed creation left events: %d, %v", count, err)
	}
}
