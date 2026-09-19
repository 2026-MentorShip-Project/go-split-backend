//go:build integration

package events

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEventFollowupContracts(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "No schedule", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	joined := a.call(t, "", "POST", "/auth/join", gin.H{"code": e.InviteCode, "name": "Guest", "email": "followup@example.com", "phone": "0912345678"}, 200)
	cookies := joined.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("missing secure session cookie")
	}
	guest := "session=" + cookies[0].Value
	// The immediate event request works with the issued cookie; missing it is 401.
	a.call(t, guest, "GET", base, nil, 200)
	a.call(t, "", "GET", base, nil, 401)
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	card := decodePRD[itemDTO](t, a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": members.Members[0].ID, "details": []gin.H{{"name": "Meal", "amount": 101}}}, 201))
	path := fmt.Sprintf("%s/items/%d", base, card.ID)
	got := decodePRD[itemDTO](t, a.call(t, host, "GET", path, nil, 200))
	d := got.Details[0]
	if len(d.CustomShares) != 0 || d.Allocation == nil || len(d.Allocation.Shares) != 2 || d.Allocation.Shares[0].Amount != 51 || d.Allocation.Shares[1].Amount != 50 {
		t.Fatalf("computed allocation missing: %+v", d)
	}
	own := decodePRD[itemDTO](t, a.call(t, guest, "GET", path, nil, 200))
	if own.Details[0].Allocation == nil || len(own.Details[0].Allocation.Shares) != 1 || own.Details[0].Allocation.Shares[0].MemberID != members.Members[1].ID {
		t.Fatal("allocation leaked other member results")
	}
	a.call(t, guest, "PATCH", base+"/settlement-note", gin.H{"note": "no"}, 403)
	a.call(t, host, "PATCH", base+"/settlement-note", gin.H{}, 400)
	a.call(t, host, "PATCH", base+"/settlement-note", gin.H{"note": ""}, 204)
	a.call(t, host, "PATCH", base+"/settlement-note", gin.H{"note": "Please transfer to the host"}, 204)
	event := a.call(t, host, "GET", base, nil, 200)
	if strings.Contains(event.Body.String(), "starts_at") || strings.Contains(event.Body.String(), "ends_at") {
		t.Fatal("scheduling exposed")
	}
	if decodePRD[eventDetailResponse](t, event).TransferNote != "Please transfer to the host" {
		t.Fatal("note missing")
	}
	a.call(t, host, "POST", base+"/settle", nil, 204)
	a.call(t, host, "PATCH", base+"/settlement-note", gin.H{"note": "changed"}, 409)
	frozen := decodePRD[itemDTO](t, a.call(t, host, "GET", path, nil, 200))
	if frozen.Details[0].Allocation.Shares[0].Amount != 51 {
		t.Fatal("snapshot allocation missing")
	}
	flows := decodePRD[personalResponse](t, a.call(t, guest, "GET", base+"/me/details", nil, 200))
	if len(flows.Transfers) != 1 || flows.Transfers[0].Amount != 50 || flows.Transfers[0].FromID != members.Members[1].ID {
		t.Fatalf("personal flow: %+v", flows)
	}
	var seeded int
	if err := a.db.QueryRow(t.Context(), `SELECT count(*) FROM templates WHERE label='自訂' AND content->'item_tags'='[]'::jsonb AND content->'cond_tags'='[]'::jsonb AND content->'rules'='[]'::jsonb`).Scan(&seeded); err != nil || seeded != 1 {
		t.Fatalf("custom seed %d: %v", seeded, err)
	}
}

func TestHostConfirmsMemberBinding(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "Binding", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	seat := decodePRD[memberDTO](t, a.call(t, host, "POST", base+"/members", gin.H{"display": "Reserved seat", "role": "co"}, 201))
	joined := a.call(t, "", "POST", "/auth/join", gin.H{"code": e.InviteCode, "name": "Actual guest", "email": "bind@example.com", "phone": "0912345678"}, 200)
	guest := "session=" + joined.Result().Cookies()[0].Value
	ms := decodePRD[membersResponse](t, a.call(t, guest, "GET", base+"/members", nil, 200))
	source := ms.Members[2].ID
	a.call(t, host, "PATCH", fmt.Sprintf("%s/members/%d", base, source), gin.H{"role": "co"}, 200)
	// Both duplicate identities already appear on expenses; binding preserves ownership and custom overrides.
	card := decodePRD[itemDTO](t, a.call(t, guest, "POST", base+"/items", gin.H{"payer_member_id": source, "details": []gin.H{{"name": "Travel", "amount": 100, "custom_amounts": gin.H{fmt.Sprint(source): 20}}, {"name": "Snack", "amount": 21, "manual_member_ids": []int64{source, seat.ID}}}}, 201))
	path := fmt.Sprintf("%s/members/%d/bind", base, seat.ID)
	a.call(t, guest, "POST", path, gin.H{"joined_member_id": source}, 403)
	bound := decodePRD[memberDTO](t, a.call(t, host, "POST", path, gin.H{"joined_member_id": source}, 200))
	if bound.ID != seat.ID || bound.Virtual || !bound.Guest || bound.Role != "co" || bound.Display != seat.Display || bound.SplitOrder != seat.SplitOrder {
		t.Fatalf("placeholder changed: %+v", bound)
	}
	event := decodePRD[eventDetailResponse](t, a.call(t, guest, "GET", base, nil, 200))
	if len(event.Members) != 2 || !event.Members[1].You || event.Members[1].ID != seat.ID {
		t.Fatalf("joined session not rebound: %+v", event.Members)
	}
	item := decodePRD[itemDTO](t, a.call(t, host, "GET", fmt.Sprintf("%s/items/%d", base, card.ID), nil, 200))
	if item.PayerMemberID != seat.ID || item.AuthorMemberID != seat.ID || item.Details[0].CustomShares[fmt.Sprint(seat.ID)] != 20 || len(item.Details[1].ManualMemberIDs) != 1 || item.Details[1].ManualMemberIDs[0] != seat.ID {
		t.Fatalf("references not moved: %+v", item)
	}
	a.call(t, host, "POST", path, gin.H{"joined_member_id": source}, 409)
	a.call(t, host, "POST", base+"/settle", nil, 204)
	a.call(t, host, "POST", path, gin.H{"joined_member_id": source}, 409)
}

func TestBindingConflictRollsBack(t *testing.T) {
	a := newPRDAPI(t)
	host := a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "Binding conflicts", "template": "自訂"}, 201))
	base := fmt.Sprintf("/events/%d", e.ID)
	seat := decodePRD[memberDTO](t, a.call(t, host, "POST", base+"/members", gin.H{"display": "Seat", "role": "member"}, 201))
	w := a.call(t, "", "POST", "/auth/join", gin.H{"code": e.InviteCode, "name": "Guest", "email": "conflict@example.com", "phone": "0912345678"}, 200)
	guest := "session=" + w.Result().Cookies()[0].Value
	ms := decodePRD[membersResponse](t, a.call(t, guest, "GET", base+"/members", nil, 200))
	source := ms.Members[2].ID
	card := decodePRD[itemDTO](t, a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": source, "details": []gin.H{{"name": "Both fixed", "amount": 100, "custom_amounts": gin.H{fmt.Sprint(source): 20, fmt.Sprint(seat.ID): 30}}}}, 201))
	bind := fmt.Sprintf("%s/members/%d/bind", base, seat.ID)
	a.call(t, host, "POST", bind, gin.H{"joined_member_id": source}, 409)
	a.call(t, host, "PATCH", fmt.Sprintf("%s/items/%d", base, card.ID), gin.H{"details": []gin.H{{"name": "Would mismatch", "amount": 100, "manual_member_ids": []int64{source, seat.ID}, "custom_amounts": gin.H{fmt.Sprint(source): 20}}}}, 200)
	a.call(t, host, "POST", bind, gin.H{"joined_member_id": source}, 422)
	event := decodePRD[eventDetailResponse](t, a.call(t, guest, "GET", base, nil, 200))
	if len(event.Members) != 3 || event.Items[0].PayerMemberID != source || !event.Members[1].Virtual {
		t.Fatal("failed binding partially committed")
	}
}
