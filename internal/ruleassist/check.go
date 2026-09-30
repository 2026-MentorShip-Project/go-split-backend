package ruleassist

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"go-split-backend/internal/rulespec"
)

// Issue codes that describe a plan rather than a single rule. Rule-level
// rejections reuse the codes rulespec already defines.
const (
	UnknownItemTag   rulespec.Code = "unknown-item-tag"
	DuplicateItemTag rulespec.Code = "duplicate-item-tag"
	UnknownMember    rulespec.Code = "unknown-member"
	InvalidOp        rulespec.Code = "invalid-op"
	InvalidLabel     rulespec.Code = "invalid-label"
)

// Issue is one reason a plan cannot be applied as written.
type Issue struct {
	ItemTag  string        `json:"item_tag,omitempty"`
	MemberID int64         `json:"member_id,omitempty"`
	Code     rulespec.Code `json:"code"`
	Detail   string        `json:"detail"`
}

// Check validates a plan against the event it would be applied to and returns
// the plan with every rule normalized, so callers preview exactly what a save
// would store. A plan with issues is still returned: the host sees what was
// understood alongside what was wrong.
//
// Tags the plan itself declares count as available, since they would be created
// in the same transaction.
func Check(p Plan, in Input) (Plan, []Issue) {
	issues := []Issue{}
	newItems := newLabels(p.NewItemTags, in.ItemTags, &issues)
	newConds := newLabels(p.NewCondTags, in.CondTags, &issues)
	items := union(in.ItemTags, newItems)
	conds := union(in.CondTags, newConds)
	members := map[int64]bool{}
	for _, m := range in.Members {
		members[m.ID] = true
	}
	existing := map[string]bool{}
	for _, r := range in.Rules {
		existing[r.ItemTag] = true
	}

	seen := map[string]bool{}
	out := p
	out.NewItemTags = newItems
	out.NewCondTags = newConds
	out.Rules = make([]PlannedRule, 0, len(p.Rules))

	for _, rule := range p.Rules {
		refusal := refuseRule(rule, items, seen)
		seen[rule.ItemTag] = true
		if refusal != nil {
			issues = append(issues, *refusal)
			continue
		}

		// A valid verb that disagrees with the event is corrected: which one
		// applies is the server's knowledge, not the generator's.
		if existing[rule.ItemTag] {
			rule.Op = Replace
		} else {
			rule.Op = Create
		}

		groups, rest, err := rulespec.NormalizeRule(rule.Groups, rule.Rest, conds)
		if err != nil {
			var refused *rulespec.Error
			if !errors.As(err, &refused) {
				issues = append(issues, Issue{ItemTag: rule.ItemTag, Code: InvalidOp, Detail: err.Error()})
				continue
			}
			issues = append(issues, Issue{ItemTag: rule.ItemTag, Code: refused.Code, Detail: refused.Detail})
			continue
		}
		rule.Groups, rule.Rest = groups, rest
		out.Rules = append(out.Rules, rule)
	}

	out.MemberConds = make([]MemberCond, 0, len(p.MemberConds))
	for _, mc := range p.MemberConds {
		if !members[mc.MemberID] {
			issues = append(issues, Issue{MemberID: mc.MemberID, Code: UnknownMember,
				Detail: fmt.Sprintf("no member with id %d", mc.MemberID)})
			continue
		}
		if err := rulespec.ValidateConditions(mc.Add, conds); err != nil {
			var refused *rulespec.Error
			code := rulespec.UnknownCond
			if errors.As(err, &refused) {
				code = refused.Code
			}
			issues = append(issues, Issue{MemberID: mc.MemberID, Code: code, Detail: err.Error()})
			continue
		}
		out.MemberConds = append(out.MemberConds, mc)
	}

	return out, issues
}

// refuseRule reports what makes a planned rule inapplicable to this event,
// before its contents are validated.
func refuseRule(rule PlannedRule, items []string, seen map[string]bool) *Issue {
	switch {
	case !contains(items, rule.ItemTag):
		return &Issue{ItemTag: rule.ItemTag, Code: UnknownItemTag,
			Detail: fmt.Sprintf("no item tag named %q", rule.ItemTag)}
	case seen[rule.ItemTag]:
		return &Issue{ItemTag: rule.ItemTag, Code: DuplicateItemTag,
			Detail: fmt.Sprintf("two rules for %q", rule.ItemTag)}
	case rule.Op != Create && rule.Op != Replace:
		return &Issue{ItemTag: rule.ItemTag, Code: InvalidOp,
			Detail: fmt.Sprintf("op must be %q or %q", Create, Replace)}
	}
	return nil
}

// newLabels keeps the proposed labels the event lacks, once each; adding one it
// already has is refused with 409. Labels the tag endpoints would refuse are
// reported instead.
func newLabels(proposed, existing []string, issues *[]Issue) []string {
	out := []string{}
	for _, v := range proposed {
		if v == "" || strings.TrimSpace(v) != v || utf8.RuneCountInString(v) > 64 {
			*issues = append(*issues, Issue{Code: InvalidLabel,
				Detail: fmt.Sprintf("label %q must be 1–64 characters without surrounding spaces", v)})
			continue
		}
		if !slices.Contains(existing, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, v := range b {
		if !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
