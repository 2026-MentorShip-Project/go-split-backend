//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	target, err := url.Parse(os.Getenv("BASE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("BASE_URL must identify the running test server")
	}
	server := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	t.Cleanup(server.Close)
	return server
}

func client(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := server.Client()
	c.Jar = jar
	c.Timeout = 10 * time.Second
	return c
}

func request(t *testing.T, c *http.Client, method, endpoint string, body any, status int, out any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != status {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: got %d, want %d: %s", method, endpoint, resp.StatusCode, status, data)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEventJourney(t *testing.T) {
	s := testServer(t)
	host, guest, outsider := client(t, s), client(t, s), client(t, s)
	account := map[string]string{"name": "CI host", "email": fmt.Sprintf("host-%d@example.com", time.Now().UnixNano()), "password": "ci-password-123"}
	request(t, host, "POST", s.URL+"/auth/register", account, 201, nil)
	request(t, host, "POST", s.URL+"/auth/logout", nil, 204, nil)
	request(t, host, "GET", s.URL+"/events", nil, 401, nil)
	request(t, host, "POST", s.URL+"/auth/login", map[string]string{"email": account["email"], "password": "wrong"}, 401, nil)
	request(t, host, "POST", s.URL+"/auth/login", account, 200, nil)
	var event struct {
		ID         int64  `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	payload := map[string]string{"name": "CI event", "template": "自訂"}
	request(t, host, "POST", s.URL+"/events", payload, 201, &event)
	if event.ID == 0 || event.InviteCode == "" {
		t.Fatal("missing event ID or invitation")
	}
	join := map[string]string{"name": "CI guest", "code": event.InviteCode, "email": account["email"], "phone": "0900000000"}
	request(t, guest, "POST", s.URL+"/auth/join", join, 200, nil)
	request(t, guest, "POST", s.URL+"/events/join", map[string]string{"code": event.InviteCode}, 200, nil)
	request(t, guest, "POST", s.URL+"/events", payload, 403, nil)
	membersURL := fmt.Sprintf("%s/events/%d/members", s.URL, event.ID)
	var members struct {
		Members []struct {
			Role string `json:"role"`
			You  bool   `json:"you"`
		} `json:"members"`
	}
	request(t, host, "GET", membersURL, nil, 200, &members)
	request(t, guest, "GET", membersURL, nil, 200, &members)
	if len(members.Members) != 2 {
		t.Fatalf("repeated join created extra members: %+v", members)
	}
	found := false
	for _, m := range members.Members {
		if m.You && m.Role == "member" {
			found = true
		}
	}
	if !found {
		t.Fatal("guest membership missing")
	}
	account["email"] = "outsider-" + account["email"]
	request(t, outsider, "POST", s.URL+"/auth/register", account, 201, nil)
	request(t, outsider, "GET", membersURL, nil, 403, nil)
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldCookies := guest.Jar.Cookies(u)
	request(t, guest, "POST", s.URL+"/auth/logout", nil, 204, nil)
	guest.Jar.SetCookies(u, oldCookies)
	request(t, guest, "GET", s.URL+"/events", nil, 401, nil)
}

func TestSmallLoad(t *testing.T) {
	if os.Getenv("RUN_LOAD") != "1" {
		t.Skip("set RUN_LOAD=1 to run k6")
	}
	s := testServer(t)
	summary, err := filepath.Abs("../../artifacts/load-summary.json")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "k6", "run", "--summary-export", summary, "../load/smoke.js")
	cmd.Env = append(os.Environ(), "LOAD_BASE_URL="+s.URL)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
