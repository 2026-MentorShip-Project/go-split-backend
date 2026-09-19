//go:build e2e

package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed identities only in the disposable E2E database. Production has no test
// login or token-verification bypass. Google verification has its own HTTP tests.
func seedAccount(t *testing.T, c *http.Client, base string, account map[string]string) string {
	t.Helper()
	var db *pgxpool.Pool
	var err error
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		db, err = pgxpool.New(t.Context(), dsn)
	} else {
		t.Fatal("TEST_DATABASE_URL must identify a disposable E2E database")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(t.Context(), "INSERT INTO accounts(name,email,google_sub) VALUES($1,$2,$3) ON CONFLICT(email) DO UPDATE SET name=EXCLUDED.name RETURNING id", account["name"], account["email"], account["email"]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(b[:])
	if _, err = db.Exec(t.Context(), "INSERT INTO sessions(token,account_id,expires_at) VALUES($1,$2,$3)", token, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	c.Jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: token, Path: "/", Secure: true}})
	return token
}
