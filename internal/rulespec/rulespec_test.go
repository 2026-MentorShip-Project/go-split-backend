package rulespec

import (
	"encoding/json"
	"errors"
	"testing"
)

var catalog = []string{"吃素", "小孩", "晚到"}

func TestNormalizeRuleRejections(t *testing.T) {
	cases := []struct {
		name   string
		groups string
		rest   string
		code   Code
		detail string
	}{
		{"not an array", `{}`, ``, InvalidGroups, "groups must be an array"},
		{"null groups", `null`, ``, InvalidGroups, "groups must be an array"},
		{"empty cond set", `[{"conds":[],"mode":"exclude"}]`, ``, EmptyCondSet, "condition set must not be empty"},
		{"cond outside catalog", `[{"conds":["未知"],"mode":"exclude"}]`, ``, UnknownCond, "unknown or duplicate condition: 未知"},
		{"repeated cond", `[{"conds":["吃素","吃素"],"mode":"exclude"}]`, ``, UnknownCond, "unknown or duplicate condition: 吃素"},
		{"duplicate set", `[{"conds":["吃素","小孩"],"mode":"exclude"},{"conds":["小孩","吃素"],"mode":"exclude"}]`, ``, DuplicateCondSet, "duplicate condition set"},
		{"bad mode", `[{"conds":["吃素"],"mode":"skip"}]`, ``, InvalidMode, "mode must be weight or exclude"},
		{"weight too small", `[{"conds":["吃素"],"mode":"weight","weight":0.05}]`, ``, InvalidWeight, "weight must be 0.1–100 with at most one decimal"},
		{"weight too large", `[{"conds":["吃素"],"mode":"weight","weight":101}]`, ``, InvalidWeight, "weight must be 0.1–100 with at most one decimal"},
		{"weight two decimals", `[{"conds":["吃素"],"mode":"weight","weight":2.55}]`, ``, InvalidWeight, "weight must be 0.1–100 with at most one decimal"},
		{"negative weight", `[{"conds":["吃素"],"mode":"weight","weight":-1}]`, ``, InvalidWeight, "weight must be 0.1–100 with at most one decimal"},
		{"rest not an object", `[]`, `"nope"`, InvalidRest, "invalid rest"},
		{"rest bad mode", `[]`, `{"mode":"skip"}`, InvalidMode, "mode must be weight or exclude"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := NormalizeRule(json.RawMessage(c.groups), json.RawMessage(c.rest), catalog)
			var got *Error
			if !errors.As(err, &got) {
				t.Fatalf("want *Error, got %v", err)
			}
			if got.Code != c.code {
				t.Errorf("code = %q, want %q", got.Code, c.code)
			}
			if got.Detail != c.detail {
				t.Errorf("detail = %q, want %q", got.Detail, c.detail)
			}
		})
	}
}

func TestNormalizeRuleDefaults(t *testing.T) {
	cases := []struct {
		name       string
		groups     string
		rest       string
		wantGroups string
		wantRest   string
	}{
		{
			"defaults an omitted rest to weight 1",
			`[{"conds":["吃素"],"mode":"exclude"}]`, ``,
			`[{"conds":["吃素"],"mode":"exclude","weight":0}]`, `{"mode":"weight","weight":1}`,
		},
		{
			"defaults an omitted weight to 1",
			`[{"conds":["小孩"],"mode":"weight"}]`, ``,
			`[{"conds":["小孩"],"mode":"weight","weight":1}]`, `{"mode":"weight","weight":1}`,
		},
		{
			"turns weight 0 into exclude",
			`[{"conds":["小孩"],"mode":"weight","weight":0}]`, ``,
			`[{"conds":["小孩"],"mode":"exclude","weight":0}]`, `{"mode":"weight","weight":1}`,
		},
		{
			"keeps an empty group list",
			`[]`, `{"mode":"exclude"}`,
			`[]`, `{"mode":"exclude","weight":0}`,
		},
		{
			"accepts one decimal place",
			`[{"conds":["小孩"],"mode":"weight","weight":0.5}]`, `{"mode":"weight","weight":1.5}`,
			`[{"conds":["小孩"],"mode":"weight","weight":0.5}]`, `{"mode":"weight","weight":1.5}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			groups, rest, err := NormalizeRule(json.RawMessage(c.groups), json.RawMessage(c.rest), catalog)
			if err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
			if string(groups) != c.wantGroups {
				t.Errorf("groups = %s, want %s", groups, c.wantGroups)
			}
			if string(rest) != c.wantRest {
				t.Errorf("rest = %s, want %s", rest, c.wantRest)
			}
		})
	}
}

func TestValidateConditions(t *testing.T) {
	if err := ValidateConditions([]string{"吃素", "小孩"}, catalog); err != nil {
		t.Fatalf("catalog members rejected: %v", err)
	}
	for _, tags := range [][]string{{"未知"}, {"吃素", "吃素"}} {
		var got *Error
		if err := ValidateConditions(tags, catalog); !errors.As(err, &got) || got.Code != UnknownCond {
			t.Errorf("%v: want UnknownCond, got %v", tags, err)
		}
	}
}
