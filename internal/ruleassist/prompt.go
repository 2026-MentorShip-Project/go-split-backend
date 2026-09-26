package ruleassist

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// systemPrompt lives beside this file so the instructions a model receives are
// reviewed and diffed like any other source.
//
//go:embed prompt.md
var systemPrompt string

// SystemPrompt returns the standing instructions: what a rule is and how the
// engine reads one. It never contains event data.
func SystemPrompt() string { return strings.TrimSpace(systemPrompt) }

// promptContext is the event, as JSON, so a model reads structured data rather
// than prose the host could shape. Member display names travel because a host
// may name someone; nothing else identifying does.
type promptContext struct {
	ItemTags []string       `json:"item_tags"`
	CondTags []string       `json:"cond_tags"`
	Rules    []ExistingRule `json:"existing_rules"`
	Members  []Member       `json:"members"`
	Request  string         `json:"request"`
}

// UserMessage renders one request. The host's words are the last field and are
// labelled as a request, not as instructions to follow: anything they write is
// a description of how to split costs, and the system prompt governs the rest.
func UserMessage(in Input) (string, error) {
	ctx := promptContext{
		ItemTags: nonNil(in.ItemTags),
		CondTags: nonNil(in.CondTags),
		Rules:    in.Rules,
		Members:  in.Members,
		Request:  in.Text,
	}
	if ctx.Rules == nil {
		ctx.Rules = []ExistingRule{}
	}
	if ctx.Members == nil {
		ctx.Members = []Member{}
	}
	body, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ResponseSchema describes the plan a model must return. Constraining the
// output is what keeps a draft referencing real tags instead of invented ones;
// Check still verifies the result, because a schema cannot express "this tag
// exists in this event".
func ResponseSchema() map[string]any {
	group := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conds":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"mode":   map[string]any{"type": "string", "enum": []string{"weight", "exclude"}},
			"weight": map[string]any{"type": "number", "minimum": 0.1, "maximum": 100},
		},
		"required": []string{"conds", "mode"},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"new_item_tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"new_cond_tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"rules": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"op":       map[string]any{"type": "string", "enum": []string{string(Create), string(Replace)}},
						"item_tag": map[string]any{"type": "string"},
						"groups":   map[string]any{"type": "array", "items": group},
						"rest":     group,
						"note":     map[string]any{"type": "string"},
					},
					"required": []string{"op", "item_tag", "groups"},
				},
			},
			"member_conds": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"member_id": map[string]any{"type": "integer"},
						"add":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"member_id", "add"},
				},
			},
			"note": map[string]any{"type": "string"},
		},
		"required": []string{"new_item_tags", "new_cond_tags", "rules", "member_conds"},
	}
}

func nonNil(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}
