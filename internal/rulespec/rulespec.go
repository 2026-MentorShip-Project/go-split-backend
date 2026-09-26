// Package rulespec validates conditional split rules. It depends on neither a
// database nor a transport, so the same checks that gate a save can run in the
// browser through the WebAssembly engine.
package rulespec

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"go-split-backend/internal/splitengine"
)

// Code identifies a rejection independently of its wording, so callers can
// translate without parsing prose.
type Code string

const (
	InvalidGroups    Code = "invalid-groups"
	InvalidRest      Code = "invalid-rest"
	EmptyCondSet     Code = "empty-cond-set"
	DuplicateCondSet Code = "duplicate-cond-set"
	UnknownCond      Code = "unknown-cond"
	InvalidMode      Code = "invalid-mode"
	InvalidWeight    Code = "invalid-weight"
)

// Error is every rejection this package returns.
type Error struct {
	Code   Code
	Detail string
}

func (e *Error) Error() string { return e.Detail }

func reject(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// InputGroup is a condition set as it arrives from a client, before defaults
// are applied. A nil Weight means the caller omitted it.
type InputGroup struct {
	Conds  []string `json:"conds"`
	Mode   string   `json:"mode"`
	Weight *float64 `json:"weight"`
}

// ValidateConditions rejects tags outside the catalog and repeated tags.
func ValidateConditions(tags, catalog []string) error {
	seen := map[string]bool{}
	for _, t := range tags {
		if !contains(catalog, t) || seen[t] {
			return reject(UnknownCond, "unknown or duplicate condition: %s", t)
		}
		seen[t] = true
	}
	return nil
}

// NormalizeRule validates a rule's groups and rest bucket and returns them in
// stored form, with defaults applied.
func NormalizeRule(groups, rest json.RawMessage, catalog []string) (json.RawMessage, json.RawMessage, error) {
	var in []InputGroup
	if err := json.Unmarshal(groups, &in); err != nil {
		return nil, nil, reject(InvalidGroups, "groups must be an array")
	}
	if in == nil {
		return nil, nil, reject(InvalidGroups, "groups must be an array")
	}
	out := []splitengine.Group{}
	sets := map[string]bool{}
	for _, g := range in {
		n, err := NormalizeGroup(g, true, catalog)
		if err != nil {
			return nil, nil, err
		}
		keyTags := append([]string(nil), g.Conds...)
		sortStrings(keyTags)
		key := strings.Join(keyTags, "\x00")
		if sets[key] {
			return nil, nil, reject(DuplicateCondSet, "duplicate condition set")
		}
		sets[key] = true
		out = append(out, n)
	}
	r := splitengine.Group{Mode: "weight", Weight: 1}
	if len(rest) > 0 && string(rest) != "null" {
		var input InputGroup
		if err := json.Unmarshal(rest, &input); err != nil {
			return nil, nil, reject(InvalidRest, "invalid rest")
		}
		var err error
		r, err = NormalizeGroup(input, false, catalog)
		if err != nil {
			return nil, nil, err
		}
	}
	gb, _ := json.Marshal(out)
	rb, _ := json.Marshal(r)
	return gb, rb, nil
}

// NormalizeGroup applies defaults and range checks. Pass condition=false for a
// rest bucket, which carries no condition set.
func NormalizeGroup(g InputGroup, condition bool, catalog []string) (splitengine.Group, error) {
	if g.Mode != "weight" && g.Mode != "exclude" {
		return splitengine.Group{}, reject(InvalidMode, "mode must be weight or exclude")
	}
	if condition {
		if len(g.Conds) == 0 {
			return splitengine.Group{}, reject(EmptyCondSet, "condition set must not be empty")
		}
		if err := ValidateConditions(g.Conds, catalog); err != nil {
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
		return splitengine.Group{}, reject(InvalidWeight, "weight must be 0.1–100 with at most one decimal")
	}
	if g.Mode == "exclude" {
		w = 0
	}
	return splitengine.Group{Conds: g.Conds, Mode: g.Mode, Weight: w}, nil
}

func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
