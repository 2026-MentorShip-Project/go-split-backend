//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// As a host
// Create an account
// Create an event using a template
// Invite three members to the event, one as a co-host and two as participants/members
//
// As a co-host
// Create an account
// Join the event as a co-host
//

func testHostCohostMemberFlow(t *testing.T, s *httptest.Server) {
	t.Helper()
	host, cohost, member1, member2 := client(t, s), client(t, s), client(t, s), client(t, s)
	stamp := time.Now().UnixNano()
	account := func(name, role string) map[string]string {
		return map[string]string{
			"name":     name,
			"email":    fmt.Sprintf("%s-%d@example.com", role, stamp),
			"password": "ci-password-123",
		}
	}
	hostAccount := account("CI host", "host")
	cohostAccount := account("CI co-host", "cohost")
	member1Account := account("CI member 1", "member1")
	member2Account := account("CI member 2", "member2")

	request(t, host, "POST", s.URL+"/auth/register", hostAccount, 201, nil)
	request(t, cohost, "POST", s.URL+"/auth/register", cohostAccount, 201, nil)
	request(t, member1, "POST", s.URL+"/auth/register", member1Account, 201, nil)
	request(t, member2, "POST", s.URL+"/auth/register", member2Account, 201, nil)

	var event struct {
		ID         int64  `json:"id"`
		InviteCode string `json:"invite_code"`
	}
	request(t, host, "POST", s.URL+"/events", map[string]string{
		"name":     "CI host event",
		"template": "烤肉/露營模板",
	}, 201, &event)
	if event.ID == 0 || event.InviteCode == "" {
		t.Fatal("event response missing id or invite code")
	}

	join := map[string]string{"code": event.InviteCode}
	request(t, cohost, "POST", s.URL+"/events/join", join, 200, nil)
	request(t, member1, "POST", s.URL+"/events/join", join, 200, nil)
	request(t, member2, "POST", s.URL+"/events/join", join, 200, nil)

	cohostID := currentMemberID(t, cohost, s.URL, event.ID)
	member1ID := currentMemberID(t, member1, s.URL, event.ID)
	request(t, host, "PATCH", fmt.Sprintf("%s/events/%d/members/%d", s.URL, event.ID, cohostID), map[string]string{
		"role": "co",
	}, 200, nil)
	request(t, host, "POST", fmt.Sprintf("%s/events/%d/tags/items", s.URL, event.ID), map[string]string{
		"label": "shared-cost",
	}, 201, nil)
	request(t, host, "POST", fmt.Sprintf("%s/events/%d/tags/conds", s.URL, event.ID), map[string]string{
		"label": "vegetarian",
	}, 201, nil)
	request(t, host, "PATCH", fmt.Sprintf("%s/events/%d/members/%d", s.URL, event.ID, member1ID), map[string]any{
		"tags": []string{"vegetarian"},
	}, 200, nil)
	request(t, host, "POST", fmt.Sprintf("%s/events/%d/rules", s.URL, event.ID), map[string]any{
		"item_tag": "shared-cost",
		"groups":   []map[string]any{{"conds": []string{"vegetarian"}, "mode": "weight", "weight": 0.5}},
		"rest":     map[string]any{"mode": "weight", "weight": 1},
	}, 201, nil)
	request(t, cohost, "POST", fmt.Sprintf("%s/events/%d/items", s.URL, event.ID), map[string]any{
		"payer_member_id": cohostID,
		"details": []map[string]any{{
			"name":         "Dinner",
			"amount_cents": 1200,
			"tag":          "shared-cost",
		}},
	}, 201, nil)
	request(t, host, "POST", fmt.Sprintf("%s/events/%d/settle", s.URL, event.ID), nil, 204, nil)

	assertMyRole(t, cohost, s.URL, event.ID, "co")
	assertMyRole(t, member1, s.URL, event.ID, "member")
	assertMyRole(t, member2, s.URL, event.ID, "member")

	var shares struct {
		GrandTotalCents int64 `json:"grand_total_cents"`
	}
	request(t, host, "GET", fmt.Sprintf("%s/events/%d/shares", s.URL, event.ID), nil, 200, &shares)
	if shares.GrandTotalCents != 1200 {
		t.Fatalf("grand total = %d, want 1200", shares.GrandTotalCents)
	}
	var transfers struct {
		Transfers []struct {
			FromID int64 `json:"from_id"`
			ToID   int64 `json:"to_id"`
			Paid   bool  `json:"paid"`
		} `json:"transfers"`
	}
	request(t, host, "GET", fmt.Sprintf("%s/events/%d/transfers", s.URL, event.ID), nil, 200, &transfers)
	if len(transfers.Transfers) == 0 {
		t.Fatal("settlement produced no transfers")
	}
	transfer := transfers.Transfers[0]
	request(t, host, "PUT", fmt.Sprintf("%s/events/%d/transfers/%d/%d/paid", s.URL, event.ID, transfer.FromID, transfer.ToID), nil, 204, nil)
	request(t, host, "GET", fmt.Sprintf("%s/events/%d/transfers", s.URL, event.ID), nil, 200, &transfers)
	if !transfers.Transfers[0].Paid {
		t.Fatal("settled transfer was not marked paid")
	}

	var members struct {
		Members []struct {
			Role string `json:"role"`
		} `json:"members"`
	}
	request(t, host, "GET", fmt.Sprintf("%s/events/%d/members", s.URL, event.ID), nil, 200, &members)
	if len(members.Members) != 4 {
		t.Fatalf("got %d event members, want 4", len(members.Members))
	}
	roles := map[string]int{}
	for _, member := range members.Members {
		roles[member.Role]++
	}
	if roles["host"] != 1 || roles["co"] != 1 || roles["member"] != 2 {
		t.Fatalf("unexpected member roles: %+v", roles)
	}
}

func currentMemberID(t *testing.T, c *http.Client, baseURL string, eventID int64) int64 {
	t.Helper()
	var response struct {
		Members []struct {
			ID  int64 `json:"id"`
			You bool  `json:"you"`
		} `json:"members"`
	}
	request(t, c, "GET", fmt.Sprintf("%s/events/%d/members", baseURL, eventID), nil, 200, &response)
	for _, member := range response.Members {
		if member.You {
			return member.ID
		}
	}
	t.Fatal("current user is missing from event members")
	return 0
}

func assertMyRole(t *testing.T, c *http.Client, baseURL string, eventID int64, want string) {
	t.Helper()
	var response struct {
		MyRole string `json:"my_role"`
	}
	request(t, c, "GET", fmt.Sprintf("%s/events/%d", baseURL, eventID), nil, 200, &response)
	if response.MyRole != want {
		t.Fatalf("event role = %q, want %q", response.MyRole, want)
	}
}
