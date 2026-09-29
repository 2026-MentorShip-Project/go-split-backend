//go:build integration

package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"go-split-backend/internal/ruleassist"

	"github.com/gin-gonic/gin"
)

const sampleDraftText = "吃素的不用分肉錢，小孩算半份"

func newDraftEvent(t *testing.T, a *prdAPI) (host, guest, base string) {
	t.Helper()
	host = a.host(t)
	e := decodePRD[createEventResponse](t, a.call(t, host, "POST", "/events", gin.H{"name": "BBQ", "template": "烤肉/露營模板"}, 201))
	join := gin.H{"code": e.InviteCode, "email": "member@example.com", "phone": "0912345678", "name": "小明"}
	guest = "session=" + a.call(t, "", "POST", "/auth/join", join, 200).Result().Cookies()[0].Value
	return host, guest, fmt.Sprintf("/events/%d", e.ID)
}

func TestRuleDraftWithoutGeneratorIsUnavailable(t *testing.T) {
	a := newPRDAPI(t)
	host, _, base := newDraftEvent(t, a)

	a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": sampleDraftText}, 503)
}

func TestRuleDraftReturnsCheckedPlanWithoutSaving(t *testing.T) {
	fixture := ruleassist.SampleFixture()
	a := newPRDAPIWithDrafter(t, fixture)
	host, guest, base := newDraftEvent(t, a)
	before := a.call(t, host, "GET", base+"/rules", nil, 200).Body.String()

	a.call(t, guest, "POST", base+"/rules/draft", gin.H{"text": sampleDraftText}, 403)
	a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": ""}, 400)
	got := decodePRD[ruleDraftResponse](t, a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": sampleDraftText}, 200))

	if len(got.Issues) != 0 || len(got.Plan.Rules) != 1 {
		t.Fatalf("plan = %+v, issues = %+v", got.Plan, got.Issues)
	}
	if len(got.Plan.NewItemTags) != 0 || len(got.Plan.NewCondTags) != 0 {
		t.Fatalf("plan re-adds labels the template already has: %v %v", got.Plan.NewItemTags, got.Plan.NewCondTags)
	}
	if got.Plan.Rules[0].Op != ruleassist.Replace {
		t.Fatalf("op = %q, want replace for the template's existing 肉品 rule", got.Plan.Rules[0].Op)
	}
	calls := fixture.Calls()
	if len(calls) != 1 || calls[0].Text != sampleDraftText || len(calls[0].Members) != 2 || len(calls[0].Rules) != 6 {
		t.Fatalf("generator input = %+v", calls)
	}
	if after := a.call(t, host, "GET", base+"/rules", nil, 200).Body.String(); after != before {
		t.Fatalf("draft changed rules:\n%s\n%s", before, after)
	}
}

func TestRuleDraftReportsCreateOnUsedTagAsLocked(t *testing.T) {
	fruit := ruleassist.Plan{Rules: []ruleassist.PlannedRule{{
		Op:      ruleassist.Create,
		ItemTag: "水果",
		Groups:  json.RawMessage(`[{"conds":["小孩"],"mode":"weight","weight":0.5}]`),
	}}}
	a := newPRDAPIWithDrafter(t, &ruleassist.FixtureGenerator{Fallback: &fruit})
	host, _, base := newDraftEvent(t, a)

	open := decodePRD[ruleDraftResponse](t, a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": "水果小孩半份"}, 200))
	if len(open.Plan.Rules) != 1 || len(open.Issues) != 0 {
		t.Fatalf("unused tag: plan = %+v, issues = %+v", open.Plan, open.Issues)
	}

	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": members.Members[0].ID, "details": []any{gin.H{"name": "Apples", "amount": 100, "tag": "水果"}}}, 201)

	locked := decodePRD[ruleDraftResponse](t, a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": "水果小孩半份"}, 200))
	if len(locked.Plan.Rules) != 0 || len(locked.Issues) != 1 || locked.Issues[0].Code != ruleLock {
		t.Fatalf("used tag: plan = %+v, issues = %+v", locked.Plan, locked.Issues)
	}

	a.call(t, host, "POST", base+"/settle", nil, 204)
	a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": "水果小孩半份"}, 409)
}

func TestRuleDraftGeneratorFailureIsBadGateway(t *testing.T) {
	a := newPRDAPIWithDrafter(t, &ruleassist.FixtureGenerator{Err: errors.New("model timeout")})
	host, _, base := newDraftEvent(t, a)

	a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": sampleDraftText}, 502)
}
