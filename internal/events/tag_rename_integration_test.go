//go:build integration

package events

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func ruleGroups(t *testing.T, a *prdAPI, cookie, base, itemTag string) string {
	t.Helper()
	rules := decodePRD[rulesResponse](t, a.call(t, cookie, "GET", base+"/rules", nil, 200))
	i := slices.IndexFunc(rules.Rules, func(r ruleDTO) bool { return r.ItemTag == itemTag })
	if i < 0 {
		t.Fatalf("no rule for %q", itemTag)
	}
	return string(rules.Rules[i].Groups)
}

func TestRenameCondTagRewritesOnlyWhatUsesIt(t *testing.T) {
	a := newPRDAPI(t)
	host, _, base := newDraftEvent(t, a)
	members := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	a.call(t, host, "PATCH", base+"/members/"+fmt.Sprint(members.Members[1].ID), gin.H{"tags": []string{"小孩"}}, 200)
	untouched := ruleGroups(t, a, host, base, "交通費")

	a.call(t, host, "PATCH", base+"/tags/conds/"+url.PathEscape("小孩"), gin.H{"label": "兒童"}, 204)

	if got := ruleGroups(t, a, host, base, "交通費"); got != untouched {
		t.Fatalf("rule without the condition changed:\n%s\n%s", untouched, got)
	}
	var meat []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ruleGroups(t, a, host, base, "肉品")), &meat); err != nil {
		t.Fatal(err)
	}
	if string(meat[1]["conds"]) != `["兒童"]` {
		t.Fatalf("肉品 group conds = %s, want renamed", meat[1]["conds"])
	}
	if _, ok := meat[0]["weight"]; ok {
		t.Fatalf("exclude group gained a weight: %v", meat[0])
	}
	after := decodePRD[membersResponse](t, a.call(t, host, "GET", base+"/members", nil, 200))
	if !slices.Equal(after.Members[1].Tags, []string{"兒童"}) {
		t.Fatalf("member tags = %v", after.Members[1].Tags)
	}
	a.call(t, host, "PATCH", base+"/tags/conds/"+url.PathEscape("兒童"), gin.H{"label": "吃素"}, 409)
}

func TestTagsWithASlashCanBeRenamedAndDeleted(t *testing.T) {
	a := newPRDAPI(t)
	host, _, base := newDraftEvent(t, a)

	a.call(t, host, "PATCH", base+"/tags/conds/"+url.PathEscape("不喝酒/開車"), gin.H{"label": "不喝酒"}, 204)
	if got := ruleGroups(t, a, host, base, "酒精飲品"); !strings.Contains(got, `"不喝酒"`) || strings.Contains(got, "開車") {
		t.Fatalf("酒精飲品 groups = %s", got)
	}

	a.call(t, host, "POST", base+"/tags/items", gin.H{"label": "租車/油資"}, 201)
	a.call(t, host, "PATCH", base+"/tags/items/"+url.PathEscape("租車/油資"), gin.H{"label": "油資/過路費"}, 204)
	a.call(t, host, "DELETE", base+"/tags/items/"+url.PathEscape("油資/過路費"), nil, 204)
	items := decodePRD[labelsResponse](t, a.call(t, host, "GET", base+"/tags/items", nil, 200))
	if slices.ContainsFunc(items.Labels, func(l string) bool { return strings.Contains(l, "/") }) {
		t.Fatalf("slash label survived: %v", items.Labels)
	}
}
