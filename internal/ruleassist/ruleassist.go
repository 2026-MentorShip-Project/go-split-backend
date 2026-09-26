// Package ruleassist drafts split rules from a host's plain description. It
// proposes only: every plan is validated against rulespec, shown to the host,
// and saved through the ordinary endpoints, which validate it again.
package ruleassist

import (
	"context"
	"encoding/json"
)

// Member is what a generator may see about one participant. Display is included
// because a host can name a person ("小明吃素"), which is the only case a
// condition may be assigned to them.
type Member struct {
	ID      int64    `json:"id"`
	Display string   `json:"display"`
	Conds   []string `json:"cond_tags"`
}

// ExistingRule summarises a rule already on the event, so a draft can replace
// one deliberately instead of colliding with it.
type ExistingRule struct {
	ItemTag string          `json:"item_tag"`
	Groups  json.RawMessage `json:"groups"`
	Rest    json.RawMessage `json:"rest,omitempty"`
}

// Input is everything a generator is given. Nothing else about the event, and
// nothing about any other event, is in scope.
type Input struct {
	Text     string         `json:"text"`
	ItemTags []string       `json:"item_tags"`
	CondTags []string       `json:"cond_tags"`
	Rules    []ExistingRule `json:"rules"`
	Members  []Member       `json:"members"`
}

// Op says what a planned rule does to the event.
type Op string

const (
	// Create adds a rule for an item tag that has none.
	Create Op = "create"
	// Replace rewrites the groups and rest of an existing rule.
	Replace Op = "replace"
)

// PlannedRule is one proposed rule, in the shape the rules API accepts, so it
// can be validated and applied without translation.
type PlannedRule struct {
	Op      Op              `json:"op"`
	ItemTag string          `json:"item_tag"`
	Groups  json.RawMessage `json:"groups"`
	Rest    json.RawMessage `json:"rest,omitempty"`
	Note    string          `json:"note,omitempty"`
}

// MemberCond assigns condition tags to a member the host named.
type MemberCond struct {
	MemberID int64    `json:"member_id"`
	Add      []string `json:"add"`
}

// Plan is a proposed set of changes. Empty lists with a Note is a valid answer
// when the request was too vague to express.
type Plan struct {
	NewItemTags []string      `json:"new_item_tags"`
	NewCondTags []string      `json:"new_cond_tags"`
	Rules       []PlannedRule `json:"rules"`
	MemberConds []MemberCond  `json:"member_conds"`
	Note        string        `json:"note,omitempty"`
}

// Generator turns a request into a plan. Implementations must not write to the
// database; drafting is read-only until the host applies the result.
type Generator interface {
	Draft(ctx context.Context, in Input) (Plan, error)
}
