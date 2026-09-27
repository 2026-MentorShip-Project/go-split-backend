//go:build integration

package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-split-backend/internal/auth"
	"go-split-backend/internal/database"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testIdentity struct{}

func (testIdentity) Verify(_ context.Context, token string) (auth.GoogleIdentity, error) {
	return auth.GoogleIdentity{Sub: token, Email: token + "@example.com", Name: token}, nil
}

type prdAPI struct {
	r  *gin.Engine
	db *pgxpool.Pool
}

func newPRDAPI(t *testing.T) *prdAPI {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database")
	}
	admin, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{fmt.Sprintf("prd_%d", time.Now().UnixNano())}.Sanitize()
	if _, err = admin.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err = SeedTemplates(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth.New(db, testIdentity{}).Register(r)
	New(db).Register(r)
	return &prdAPI{r, db}
}
func (a *prdAPI) call(t *testing.T, cookie, method, path string, body any, status int) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	a.r.ServeHTTP(w, req)
	if w.Code != status {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body)
	}
	return w
}
func decodePRD[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func (a *prdAPI) host(t *testing.T) string {
	t.Helper()
	w := a.call(t, "", "POST", "/auth/google", gin.H{"id_token": "host"}, 200)
	return "session=" + w.Result().Cookies()[0].Value
}
func TestPRDLifecycle(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	a.call(t, "", "POST", "/auth/register", gin.H{}, 400)
	a.call(t, "", "POST", "/auth/login", gin.H{}, 400)
	pw := gin.H{"name": "Password user", "email": "password@example.com", "password": "password123"}
	a.call(t, "", "POST", "/auth/register", pw, 201)
	a.call(t, "", "POST", "/auth/register", pw, 409)
	a.call(t, "", "POST", "/auth/login", gin.H{"email": pw["email"], "password": "wrong-password"}, 401)
	a.call(t, "", "POST", "/auth/login", gin.H{"email": pw["email"], "password": pw["password"]}, 200)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "PRD", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	a.call(t, "", "GET", "/auth/invite/"+e.InviteCode, nil, 200)
	a.call(t, host, "POST", base+"/archive", nil, 409)
	join := gin.H{"code": e.InviteCode, "email": "member@example.com", "phone": "0912345678", "name": "member"}
	w := a.call(t, "", "POST", "/auth/join", join, 200)
	guest := "session=" + w.Result().Cookies()[0].Value
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	hid, mid := members.Members[0].ID, members.Members[1].ID
	a.call(t, host, "PATCH", fmt.Sprintf("%s/members/%d", base, mid), gin.H{"role": "host"}, 409)
	a.call(t, host, "PATCH", fmt.Sprintf("%s/members/%d", base, hid), gin.H{"role": "member"}, 409)
	a.call(t, host, "DELETE", fmt.Sprintf("%s/members/%d", base, hid), nil, 409)
	co := decodePRD[memberDTO](t, a.call(t, host, "POST", base+"/members", gin.H{"display": "co", "role": "co"}, 201))
	a.call(t, guest, "POST", base+"/items", gin.H{"payer_member_id": mid, "details": []any{}}, 403)
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{}}, 201)
	a.call(t, host, "POST", base+"/apply-template", gin.H{"label": "烤肉/露營模板"}, 404)
	a.call(t, host, "PATCH", base, gin.H{"name": "Updated", "template": "烤肉/露營模板"}, 409)
	a.call(t, host, "PATCH", base, gin.H{"name": "Updated"}, 204)
	for _, detail := range []gin.H{
		{"name": "bad", "amount": 300, "manual_member_ids": []int64{}},
		{"name": "bad", "amount": 300, "custom_amounts": gin.H{fmt.Sprint(hid): 350}},
		{"name": "bad", "amount": 300, "custom_amounts": gin.H{fmt.Sprint(hid): 100, fmt.Sprint(mid): 100, fmt.Sprint(co.ID): 50}},
		{"name": " ", "amount": 300},
	} {
		a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{gin.H{"name": "valid", "amount": 30}, detail}}, 422)
	}

	for _, detail := range []gin.H{{"name": "missing"}, {"name": "legacy", "amount_cents": 100}, {"name": "null", "amount": nil}} {
		a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{detail}}, 400)
	}
	before := decodePRD[itemsResponse](t, a.call(t, host, "GET", base+"/items", nil, 200))
	if len(before.Items) != 1 {
		t.Fatal("invalid card partially committed")
	}
	card := decodePRD[itemDTO](t, a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": co.ID, "details": []any{gin.H{"name": "Dinner", "amount": 300}}}, 201))
	a.call(t, host, "PATCH", fmt.Sprintf("%s/items/%d", base, card.ID), gin.H{"details": []any{gin.H{"name": "", "amount": 300}}}, 422)
	a.call(t, host, "DELETE", fmt.Sprintf("%s/members/%d", base, mid), nil, 409)
	a.call(t, guest, "GET", base+"/transfers", nil, 403)
	personal := decodePRD[sharesResponse](t, a.call(t, guest, "GET", base+"/shares", nil, 200))
	if len(personal.PerMember) != 1 || personal.PerMember[0].MemberID != mid {
		t.Fatal("personal visibility")
	}
	a.call(t, host, "GET", base+"/transfers?mode=min", nil, 400)
	ts := decodePRD[transfersResponse](t, a.call(t, host, "GET", base+"/transfers", nil, 200))
	if len(ts.Transfers) != 2 {
		t.Fatalf("hub transfers %+v", ts)
	}
	for _, tr := range ts.Transfers {
		if tr.FromID != hid && tr.ToID != hid {
			t.Fatal("hub bypass")
		}
	}
	a.call(t, guest, "POST", base+"/settle", nil, 403)
	a.call(t, host, "POST", base+"/settle", nil, 204)
	snap, err := loadSnapshot(t.Context(), a.db, e.ID)
	if err != nil || snap == nil || snap.HubID != hid || snap.EngineVersion == "" || snap.Strategy != "hub" {
		t.Fatalf("snapshot %v %v", snap, err)
	}
	if len(snap.Event.Items) != 2 || snap.Engine.Shares.GrandTotal != 300 {
		t.Fatal("incomplete snapshot")
	}
	for _, route := range []struct {
		method, path string
		body         any
	}{{"POST", "/items", gin.H{"payer_member_id": hid}}, {"PATCH", fmt.Sprintf("/items/%d", card.ID), gin.H{"details": []any{}}}, {"DELETE", fmt.Sprintf("/items/%d", card.ID), nil}, {"PATCH", fmt.Sprintf("/members/%d", mid), gin.H{"tags": []string{}}}, {"POST", "/rules", gin.H{}}, {"POST", "/tags/items", gin.H{"label": "late"}}, {"PATCH", "", gin.H{"name": "late"}}} {
		a.call(t, host, route.method, base+route.path, route.body, 409)
	}
	a.call(t, "", "GET", "/auth/invite/"+e.InviteCode, nil, 410)
	a.call(t, "", "POST", "/auth/join", join, 410)
	a.call(t, "", "POST", "/auth/recover", join, 410)
	lines := decodePRD[personalResponse](t, a.call(t, guest, "GET", base+"/me/details", nil, 200))
	if len(lines.Lines) != 1 || lines.Net != -100 || lines.Lines[0].Net != -100 {
		t.Fatalf("personal lines %+v", lines)
	}
	if _, err = a.db.Exec(t.Context(), "UPDATE item_details SET amount=999 WHERE item_id=$1", card.ID); err == nil {
		t.Fatal("direct write after settlement accepted")
	}
	a.call(t, host, "POST", base+"/archive", nil, 204)
	a.call(t, host, "POST", base+"/archive", nil, 204)
	a.call(t, guest, "GET", base+"/transfers", nil, 200)
	got := decodePRD[sharesResponse](t, a.call(t, host, "GET", base+"/shares", nil, 200))
	if got.GrandTotal != 300 {
		t.Fatal("snapshot drift")
	}
	a.call(t, host, "PUT", fmt.Sprintf("%s/transfers/%d/%d/paid", base, mid, hid), nil, 404)
}
func TestPRDRuleLocks(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "Rules", "template": "烤肉/露營模板"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	ms := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	hid := ms.Members[0].ID
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{gin.H{"name": "No riders", "amount": 1200, "tag": "交通費"}}}, 422)
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{gin.H{"name": "Rule bypass", "amount": 100, "tag": "肉品", "custom_amounts": gin.H{fmt.Sprint(hid): 100}}}}, 422)
	a.call(t, host, "PATCH", fmt.Sprintf("%s/members/%d", base, hid), gin.H{"tags": []string{"大人"}}, 200)
	a.call(t, host, "DELETE", base+"/tags/conds/大人", nil, 409)
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": hid, "details": []any{gin.H{"name": "Meat", "amount": 100, "tag": "肉品"}, gin.H{"name": "Fruit", "amount": 10, "tag": "水果"}}}, 201)
	rules := decodePRD[rulesResponse](t, a.call(t, host, "GET", base+"/rules", nil, 200))
	rid := rules.Rules[0].ID
	ruleURL := fmt.Sprintf("%s/rules/%d", base, rid)
	conflict := decodePRD[struct {
		DetailsURL string `json:"details_url"`
	}](t, a.call(t, host, "DELETE", ruleURL, nil, 409))
	found := decodePRD[itemsResponse](t, a.call(t, host, "GET", conflict.DetailsURL, nil, 200))
	if len(found.Items) != 1 {
		t.Fatal("rule usage link did not locate card")
	}
	absent := decodePRD[itemsResponse](t, a.call(t, host, "GET", base+"/items?tag=absent", nil, 200))
	if len(absent.Items) != 0 {
		t.Fatal("tag filter returned unrelated cards")
	}
	a.call(t, host, "DELETE", base+"/tags/items/肉品", nil, 409)
	a.call(t, host, "POST", base+"/rules", gin.H{"item_tag": "水果", "groups": []any{}}, 409)
	for _, weight := range []float64{.01, 2.55, 101, -1} {
		a.call(t, host, "PATCH", ruleURL, gin.H{"rest": gin.H{"mode": "weight", "weight": weight}}, 400)
	}
	a.call(t, host, "PATCH", ruleURL, gin.H{"rest": gin.H{"mode": "weight", "weight": 2}}, 200)
	a.call(t, host, "PATCH", base+"/tags/items/肉品", gin.H{"label": "肉類"}, 204)
	items := decodePRD[itemsResponse](t, a.call(t, host, "GET", base+"/items", nil, 200))
	if *items.Items[0].Details[0].Tag != "肉類" {
		t.Fatal("rename disconnected expenses")
	}
	a.call(t, host, "PATCH", ruleURL, gin.H{"rest": gin.H{"mode": "weight", "weight": 0}}, 200)
	a.call(t, host, "POST", base+"/settle", nil, 422)
	a.call(t, host, "PATCH", ruleURL, gin.H{"rest": gin.H{"mode": "weight"}}, 200)
	a.call(t, host, "POST", base+"/settle", nil, 204)
}
func TestPRDConcurrentSettlement(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "Concurrent", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	ms := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	hid := ms.Members[0].ID
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, path := range []string{"/settle", "/items"} {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			body := fmt.Sprintf(`{"payer_member_id":%d,"details":[{"name":"Race","amount":100}]}`, hid)
			req := httptest.NewRequestWithContext(t.Context(), "POST", base+path, strings.NewReader(body))
			req.Header.Set("Cookie", host)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			a.r.ServeHTTP(w, req)
			statuses <- w.Code
		}(path)
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 201 && status != 204 && status != 409 {
			t.Fatalf("race status %d", status)
		}
	}
	snap, err := loadSnapshot(t.Context(), a.db, e.ID)
	if err != nil || snap == nil {
		t.Fatalf("snapshot %v", err)
	}
	var sum int64
	for _, it := range snap.Event.Items {
		sum += it.Total
	}
	if sum != snap.Engine.Shares.GrandTotal {
		t.Fatal("torn snapshot")
	}
}

func TestPRDDatabaseHostInvariant(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "host constraints", "template": "自訂"}, 201))
	statements := []string{
		"UPDATE event_members SET role='member' WHERE event_id=$1 AND role='host'",
		"DELETE FROM event_members WHERE event_id=$1 AND role='host'",
		"INSERT INTO event_members(event_id,display,role) VALUES($1,'guest host','host')",
		"INSERT INTO event_members(event_id,display,role,account_id) SELECT id,'second host','host',account_id FROM events WHERE id=$1",
	}
	for _, sql := range statements {
		if _, err := a.db.Exec(t.Context(), sql, e.ID); err == nil {
			t.Fatalf("host constraint allowed %s", sql)
		}
	}
}

func TestPRDStableDetailIDs(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "stable details", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	card := decodePRD[itemDTO](t, a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": members.Members[0].ID, "details": []gin.H{{"name": "A", "amount": 10}, {"name": "B", "amount": 20}}}, 201))
	path := fmt.Sprintf("%s/items/%d", base, card.ID)
	swapped := []gin.H{{"id": card.Details[1].ID, "name": "B", "amount": 21}, {"id": card.Details[0].ID, "name": "A", "amount": 11}}
	got := decodePRD[itemDTO](t, a.call(t, host, "PATCH", path, gin.H{"details": swapped}, 200))
	if got.Details[0].ID != card.Details[1].ID || got.Details[1].ID != card.Details[0].ID || got.Total != 32 {
		t.Fatalf("identities changed: %+v", got)
	}
	swapped[1]["id"] = card.Details[1].ID
	a.call(t, host, "PATCH", path, gin.H{"details": swapped}, 400)
	a.call(t, host, "PATCH", path, gin.H{"details": []gin.H{{"id": int64(999999), "name": "foreign", "amount": 3}}}, 400)
	a.call(t, host, "PATCH", path, gin.H{"details": []gin.H{}}, 200)
}
