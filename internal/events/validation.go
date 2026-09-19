package events

import (
	"context"
	"encoding/json"
	"fmt"
	"go-split-backend/internal/database"
	"go-split-backend/internal/splitengine"
	"math"
	"strconv"
	"strings"
)

type detailIssue struct {
	ItemID   int64  `json:"item_id,omitempty"`
	DetailID int64  `json:"detail_id,omitempty"`
	Index    int    `json:"index"`
	Code     string `json:"code"`
	Diff     int64  `json:"diff,omitempty"`
}
type validationResponse struct {
	Error   string        `json:"error"`
	Details []detailIssue `json:"details"`
}

func splitIssues(r engineResult) []detailIssue {
	out := []detailIssue{}
	for i, d := range r.Shares.PerDetail {
		if d.Result.Validity != splitengine.OK {
			out = append(out, detailIssue{d.ItemID, d.DetailID, i, string(d.Result.Validity), d.Result.Diff})
		}
	}
	return out
}
func validateDetails(ctx context.Context, db database.Store, id int64, details []createDetailRequest) ([]detailIssue, error) {
	ms, err := loadEngineMembers(ctx, db, id)
	if err != nil {
		return nil, err
	}
	rules, err := loadEngineRules(ctx, db, id)
	if err != nil {
		return nil, err
	}
	tags, err := loadTagLabels(ctx, db, id, "event_item_tags")
	if err != nil {
		return nil, err
	}
	out := []detailIssue{}
	for i, d := range details {
		code, tag := validateDetailInput(d, ms, rules, tags)
		if code != "" {
			out = append(out, detailIssue{Index: i, Code: code})
			continue
		}
		r := splitengine.SplitDetail(splitengine.Detail{Amount: d.Amount, Tag: tag, ManualMemberIDs: d.ManualMemberIDs, CustomShares: numericStringMap(d.CustomShares)}, ms, rules, nil, 1)
		if r.Validity != splitengine.OK {
			out = append(out, detailIssue{Index: i, Code: string(r.Validity), Diff: r.Diff})
		}
	}
	return out, nil
}
func validateDetailInput(d createDetailRequest, ms []splitengine.Member, rules []splitengine.Rule, tags []string) (string, string) {
	code := ""
	tag := ""
	if d.Tag != nil {
		tag = *d.Tag
	}
	switch {
	case strings.TrimSpace(d.Name) == "" || len(d.Name) > 120:
		code = "invalid-name"
	case d.Amount < 0 || d.Amount > 1_000_000_000_000:
		code = "invalid-amount"
	case tag != "" && !stringIn(tags, tag):
		code = "unknown-item-tag"
	}
	ruled := false
	for _, r := range rules {
		if r.Tag == tag {
			ruled = true
		}
	}
	if ruled && (d.ManualMemberIDs != nil || len(d.CustomShares) > 0) {
		code = "rule-lock"
	}
	pool := map[int64]bool{}
	for _, m := range ms {
		pool[m.ID] = true
	}
	manual := map[int64]bool{}
	for _, mid := range d.ManualMemberIDs {
		if !pool[mid] || manual[mid] {
			code = "invalid-participant"
		}
		manual[mid] = true
	}
	for key, amount := range d.CustomShares {
		mid, e := strconv.ParseInt(key, 10, 64)
		if e != nil || strconv.FormatInt(mid, 10) != key || !pool[mid] || amount < 0 || amount > 1_000_000_000_000 || (d.ManualMemberIDs != nil && !manual[mid]) {
			code = "invalid-custom-amount"
		}
	}
	return code, tag
}
func stringIn(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func validateConditions(tags, catalog []string) error {
	seen := map[string]bool{}
	for _, t := range tags {
		if !stringIn(catalog, t) || seen[t] {
			return fmt.Errorf("unknown or duplicate condition: %s", t)
		}
		seen[t] = true
	}
	return nil
}
func normalizeRule(groups, rest json.RawMessage, catalog []string) (json.RawMessage, json.RawMessage, error) {
	var in []inputGroup
	if err := json.Unmarshal(groups, &in); err != nil {
		return nil, nil, fmt.Errorf("groups must be an array")
	}
	if in == nil {
		return nil, nil, fmt.Errorf("groups must be an array")
	}
	out := []splitengine.Group{}
	sets := map[string]bool{}
	for _, g := range in {
		n, err := normalizeGroup(g, true, catalog)
		if err != nil {
			return nil, nil, err
		}
		keyTags := append([]string(nil), g.Conds...)
		sortStrings(keyTags)
		key := strings.Join(keyTags, "\x00")
		if sets[key] {
			return nil, nil, fmt.Errorf("duplicate condition set")
		}
		sets[key] = true
		out = append(out, n)
	}
	r := splitengine.Group{Mode: "weight", Weight: 1}
	if len(rest) > 0 && string(rest) != "null" {
		var input inputGroup
		if err := json.Unmarshal(rest, &input); err != nil {
			return nil, nil, fmt.Errorf("invalid rest")
		}
		var err error
		r, err = normalizeGroup(input, false, catalog)
		if err != nil {
			return nil, nil, err
		}
	}
	gb, _ := json.Marshal(out)
	rb, _ := json.Marshal(r)
	return gb, rb, nil
}
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

type inputGroup struct {
	Conds  []string `json:"conds"`
	Mode   string   `json:"mode"`
	Weight *float64 `json:"weight"`
}

func normalizeGroup(g inputGroup, condition bool, catalog []string) (splitengine.Group, error) {
	if g.Mode != "weight" && g.Mode != "exclude" {
		return splitengine.Group{}, fmt.Errorf("mode must be weight or exclude")
	}
	if condition {
		if len(g.Conds) == 0 {
			return splitengine.Group{}, fmt.Errorf("condition set must not be empty")
		}
		if err := validateConditions(g.Conds, catalog); err != nil {
			return splitengine.Group{}, err
		}
	}
	w := 1.0
	if g.Weight != nil {
		w = *g.Weight
	}
	if w == 0 {
		g.Mode = "exclude"
	}
	if g.Mode == "weight" && (w < .1 || w > 100 || math.Abs(w*10-math.Round(w*10)) > 1e-8) {
		return splitengine.Group{}, fmt.Errorf("weight must be 0.1–100 with at most one decimal")
	}
	if g.Mode == "exclude" {
		w = 0
	}
	return splitengine.Group{Conds: g.Conds, Mode: g.Mode, Weight: w}, nil
}
