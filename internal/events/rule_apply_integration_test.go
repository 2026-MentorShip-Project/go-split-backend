//go:build integration

package events

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"go-split-backend/internal/ruleassist"

	"github.com/gin-gonic/gin"
)

func TestRuleApplySavesTheDraftedPlan(t *testing.T) {
	a := newPRDAPIWithDrafter(t, ruleassist.SampleFixture())
	host, _, base := newDraftEvent(t, a)
	draft := decodePRD[ruleDraftResponse](t, a.call(t, host, "POST", base+"/rules/draft", gin.H{"text": sampleDraftText}, 200))

	a.call(t, host, "POST", base+"/rules/apply", draft.Plan, 204)

	rules := decodePRD[rulesResponse](t, a.call(t, host, "GET", base+"/rules", nil, 200))
	i := slices.IndexFunc(rules.Rules, func(r ruleDTO) bool { return r.ItemTag == "肉品" })
	if i < 0 || !jsonEqual(t, rules.Rules[i].Groups, draft.Plan.Rules[0].Groups) {
		t.Fatalf("肉品 rule = %+v, want groups %s", rules.Rules, draft.Plan.Rules[0].Groups)
	}
}

func TestRuleApplyWritesTagsRulesAndMemberConditionsTogether(t *testing.T) {
	a := newPRDAPI(t)
	host, _, base := newDraftEvent(t, a)
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	guest := members.Members[1]
	a.call(t, host, "PATCH", base+"/members/"+fmt.Sprint(guest.ID), gin.H{"tags": []string{"大人"}}, 200)

	plan := ruleassist.Plan{
		NewItemTags: []string{"飲料"},
		NewCondTags: []string{"不吃辣"},
		Rules: []ruleassist.PlannedRule{{
			Op: ruleassist.Create, ItemTag: "飲料",
			Groups: json.RawMessage(`[{"conds":["不吃辣"],"mode":"exclude"}]`),
		}},
		MemberConds: []ruleassist.MemberCond{{MemberID: guest.ID, Add: []string{"不吃辣", "大人"}}},
	}
	a.call(t, host, "POST", base+"/rules/apply", plan, 204)

	items := decodePRD[labelsResponse](t, a.call(t, host, "GET", base+"/tags/items", nil, 200))
	conds := decodePRD[labelsResponse](t, a.call(t, host, "GET", base+"/tags/conds", nil, 200))
	if !slices.Contains(items.Labels, "飲料") || !slices.Contains(conds.Labels, "不吃辣") {
		t.Fatalf("labels not added: items %v conds %v", items.Labels, conds.Labels)
	}
	rules := decodePRD[rulesResponse](t, a.call(t, host, "GET", base+"/rules", nil, 200))
	if !slices.ContainsFunc(rules.Rules, func(r ruleDTO) bool { return r.ItemTag == "飲料" }) {
		t.Fatalf("飲料 rule missing: %+v", rules.Rules)
	}
	after := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	if got := after.Members[1].Tags; !slices.Equal(got, []string{"大人", "不吃辣"}) {
		t.Fatalf("member tags = %v, want existing tags kept and 不吃辣 added once", got)
	}
}

func TestRuleApplyWritesNothingWhenAnyPartNoLongerApplies(t *testing.T) {
	a := newPRDAPI(t)
	host, guestSession, base := newDraftEvent(t, a)
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	a.call(t, host, "POST", base+"/items", gin.H{"payer_member_id": members.Members[0].ID, "details": []any{gin.H{"name": "Apples", "amount": 100, "tag": "水果"}}}, 201)
	before := a.call(t, host, "GET", base+"/tags/items", nil, 200).Body.String()

	withNewTag := func(r ruleassist.PlannedRule) ruleassist.Plan {
		return ruleassist.Plan{NewItemTags: []string{"飲料"}, Rules: []ruleassist.PlannedRule{r}}
	}
	cases := []struct {
		name string
		plan ruleassist.Plan
		code string
	}{
		{"create on a tag expenses use", withNewTag(ruleassist.PlannedRule{Op: ruleassist.Create, ItemTag: "水果", Groups: json.RawMessage(`[]`)}), string(ruleLock)},
		{"create where a rule now exists", withNewTag(ruleassist.PlannedRule{Op: ruleassist.Create, ItemTag: "肉品", Groups: json.RawMessage(`[]`)}), string(ruleassist.InvalidOp)},
		{"condition for a stranger", ruleassist.Plan{NewItemTags: []string{"飲料"}, MemberConds: []ruleassist.MemberCond{{MemberID: 999999, Add: []string{"吃素"}}}}, string(ruleassist.UnknownMember)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodePRD[ruleApplyRejection](t, a.call(t, host, "POST", base+"/rules/apply", c.plan, 422))
			if len(got.Issues) != 1 || string(got.Issues[0].Code) != c.code {
				t.Fatalf("issues = %+v, want one %s", got.Issues, c.code)
			}
			if after := a.call(t, host, "GET", base+"/tags/items", nil, 200).Body.String(); after != before {
				t.Fatalf("rejected plan still added labels:\n%s\n%s", before, after)
			}
		})
	}

	a.call(t, guestSession, "POST", base+"/rules/apply", ruleassist.Plan{}, 403)
	a.call(t, host, "POST", base+"/settle", nil, 204)
	a.call(t, host, "POST", base+"/rules/apply", ruleassist.Plan{}, 409)
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(ax) == string(by)
}
