//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	capacityEvents       = 20
	capacityMembers      = 5
	capacityItems        = 10
	capacitySettleEvents = 40
)

type capacityEvent struct {
	ID      int64    `json:"id"`
	Host    string   `json:"host"`
	Payer   int64    `json:"payer"`
	Members []string `json:"members,omitempty"`
}

type capacityFixture struct {
	Events []capacityEvent `json:"events"`
	Hot    capacityEvent   `json:"hot"`
	Settle []capacityEvent `json:"settle"`
}

// TestCapacity seeds realistic events, then drives tests/load/capacity.js
// against BASE_URL directly, so the TLS proxy is not part of the measurement.
func TestCapacity(t *testing.T) {
	if os.Getenv("RUN_CAPACITY") != "1" {
		t.Skip("set RUN_CAPACITY=1 to run k6")
	}
	s := testServer(t)
	stamp := time.Now().UnixNano()
	var f capacityFixture
	for i := range capacityEvents {
		f.Events = append(f.Events, seedCapacityEvent(t, s, fmt.Sprintf("read-%d-%d", stamp, i), capacityMembers, capacityItems))
	}
	f.Hot = seedCapacityEvent(t, s, fmt.Sprintf("hot-%d", stamp), 0, 0)
	for i := range capacitySettleEvents {
		f.Settle = append(f.Settle, seedCapacityEvent(t, s, fmt.Sprintf("settle-%d-%d", stamp, i), 1, 2))
	}

	artifacts, err := filepath.Abs("../../artifacts")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artifacts, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(artifacts, "capacity-fixture.json")
	if err := os.WriteFile(fixture, data, 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), "k6", "run", "--summary-export", filepath.Join(artifacts, "capacity-summary.json"), "../load/capacity.js")
	cmd.Env = append(os.Environ(), "LOAD_BASE_URL="+os.Getenv("BASE_URL"), "LOAD_FIXTURE="+fixture)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func seedCapacityEvent(t *testing.T, srv *httptest.Server, name string, members, items int) capacityEvent {
	t.Helper()
	base := srv.URL
	host := client(t, srv)
	token := seedAccount(t, host, base, map[string]string{"name": "Host " + name, "email": name + "@example.com"})
	var event struct {
		ID         int64  `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	request(t, host, "POST", base+"/events", map[string]string{"name": "Load " + name, "template": "烤肉/露營模板"}, 201, &event)
	e := capacityEvent{ID: event.ID, Host: token, Payer: currentMemberID(t, host, base, event.ID)}
	for i := range members {
		guest := client(t, srv)
		join := map[string]string{"code": event.InviteCode, "name": fmt.Sprintf("Guest %d", i), "email": fmt.Sprintf("guest-%d-%s@example.com", i, name), "phone": "0912345678"}
		request(t, guest, "POST", base+"/auth/join", join, 200, nil)
		e.Members = append(e.Members, sessionToken(t, guest, base))
	}
	for i := range items {
		request(t, host, "POST", fmt.Sprintf("%s/events/%d/items", base, event.ID), capacityItem(e.Payer, i), 201, nil)
	}
	return e
}

// capacityItem mirrors itemBody in tests/load/capacity.js: one ruled line and
// one untagged line, so every save runs the rule engine.
func capacityItem(payer int64, n int) map[string]any {
	return map[string]any{
		"payer_member_id": payer,
		"details": []map[string]any{
			{"name": "肉", "amount": 300 + n, "tag": "肉品"},
			{"name": "飲料", "amount": 120, "tag": nil},
		},
	}
}

func sessionToken(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range c.Jar.Cookies(u) {
		if cookie.Name == "session" {
			return cookie.Value
		}
	}
	t.Fatal("join issued no session cookie")
	return ""
}
