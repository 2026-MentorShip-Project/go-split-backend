package ruleassist

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"go-split-backend/internal/rulespec"

	"google.golang.org/genai"
)

func event() Input {
	return Input{
		ItemTags: []string{"肉品", "水果"},
		CondTags: []string{"吃素", "小孩"},
		Rules:    []ExistingRule{{ItemTag: "水果", Groups: json.RawMessage(`[]`)}},
		Members:  []Member{{ID: 1, Display: "小凱"}, {ID: 2, Display: "阿豪", Conds: []string{"小孩"}}},
	}
}

func rule(tag string, groups string) PlannedRule {
	return PlannedRule{Op: Create, ItemTag: tag, Groups: json.RawMessage(groups)}
}

func TestCheckRefusesPlansTheEventCannotHold(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
		code rulespec.Code
		// kept is how many rules legitimately survive; a duplicate refuses the
		// second occurrence and keeps the first.
		kept int
	}{
		{"item tag that does not exist", Plan{Rules: []PlannedRule{rule("海鮮", `[]`)}}, UnknownItemTag, 0},
		{"two rules for one tag", Plan{Rules: []PlannedRule{rule("肉品", `[]`), rule("肉品", `[]`)}}, DuplicateItemTag, 1},
		{"op outside the enum", Plan{Rules: []PlannedRule{{Op: "delete", ItemTag: "肉品", Groups: json.RawMessage(`[]`)}}}, InvalidOp, 0},
		{"condition outside the catalog", Plan{Rules: []PlannedRule{rule("肉品", `[{"conds":["海鮮過敏"],"mode":"exclude"}]`)}}, rulespec.UnknownCond, 0},
		{"weight the engine cannot store", Plan{Rules: []PlannedRule{rule("肉品", `[{"conds":["小孩"],"mode":"weight","weight":2.55}]`)}}, rulespec.InvalidWeight, 0},
		{"group with no conditions", Plan{Rules: []PlannedRule{rule("肉品", `[{"conds":[],"mode":"exclude"}]`)}}, rulespec.EmptyCondSet, 0},
		{"condition for a stranger", Plan{MemberConds: []MemberCond{{MemberID: 99, Add: []string{"吃素"}}}}, UnknownMember, 0},
		{"condition outside the catalog for a member", Plan{MemberConds: []MemberCond{{MemberID: 1, Add: []string{"未知"}}}}, rulespec.UnknownCond, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, issues := Check(c.plan, event())
			if len(issues) != 1 {
				t.Fatalf("issues = %+v, want exactly one %s", issues, c.code)
			}
			if issues[0].Code != c.code {
				t.Errorf("code = %q, want %q", issues[0].Code, c.code)
			}
			if len(out.Rules) != c.kept {
				t.Errorf("kept %d rules, want %d: %+v", len(out.Rules), c.kept, out.Rules)
			}
			if len(out.MemberConds) != 0 {
				t.Errorf("refused member conditions survived: %+v", out.MemberConds)
			}
		})
	}
}

func TestCheckNormalizesAndCorrectsOp(t *testing.T) {
	plan := Plan{
		NewItemTags: []string{"海鮮"},
		NewCondTags: []string{"海鮮過敏"},
		Rules: []PlannedRule{
			// Declared tags count as available, since they land in the same save.
			rule("海鮮", `[{"conds":["海鮮過敏"],"mode":"exclude"}]`),
			// Says create, but the event already has a 水果 rule.
			rule("水果", `[{"conds":["小孩"],"mode":"weight","weight":0.5}]`),
		},
		MemberConds: []MemberCond{{MemberID: 2, Add: []string{"小孩"}}},
	}

	out, issues := Check(plan, event())
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(out.Rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(out.Rules))
	}
	if out.Rules[0].Op != Create || out.Rules[1].Op != Replace {
		t.Errorf("ops = %q, %q; want create, replace", out.Rules[0].Op, out.Rules[1].Op)
	}
	// Normalized, so a preview shows what would actually be stored.
	if string(out.Rules[0].Groups) != `[{"conds":["海鮮過敏"],"mode":"exclude","weight":0}]` {
		t.Errorf("groups not normalized: %s", out.Rules[0].Groups)
	}
	if string(out.Rules[0].Rest) != `{"mode":"weight","weight":1}` {
		t.Errorf("rest not defaulted: %s", out.Rules[0].Rest)
	}
	if len(out.MemberConds) != 1 {
		t.Errorf("member conds dropped: %+v", out.MemberConds)
	}
}

func TestCheckKeepsGoodRulesWhenOneIsBad(t *testing.T) {
	plan := Plan{Rules: []PlannedRule{
		rule("肉品", `[{"conds":["吃素"],"mode":"exclude"}]`),
		rule("水果", `[{"conds":["未知"],"mode":"exclude"}]`),
	}}
	out, issues := Check(plan, event())
	if len(issues) != 1 || issues[0].ItemTag != "水果" {
		t.Fatalf("issues = %+v", issues)
	}
	if len(out.Rules) != 1 || out.Rules[0].ItemTag != "肉品" {
		t.Fatalf("good rule not kept: %+v", out.Rules)
	}
}

func TestCheckDropsLabelsTheEventAlreadyHas(t *testing.T) {
	plan := Plan{NewItemTags: []string{"肉品", "飲料", "飲料"}, NewCondTags: []string{"小孩", "吃素"}}

	out, issues := Check(plan, event())

	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if !slices.Equal(out.NewItemTags, []string{"飲料"}) || len(out.NewCondTags) != 0 {
		t.Fatalf("new item tags = %v, new cond tags = %v", out.NewItemTags, out.NewCondTags)
	}
}

func TestFixtureGeneratorRecordsAndAnswers(t *testing.T) {
	want := Plan{Note: "matched"}
	other := Plan{Note: "fallback"}
	g := &FixtureGenerator{Plans: map[string]Plan{"hello": want}, Fallback: &other}

	got, err := g.Draft(context.Background(), Input{Text: "hello"})
	if err != nil || got.Note != "matched" {
		t.Fatalf("keyed plan: %+v %v", got, err)
	}
	if got, _ = g.Draft(context.Background(), Input{Text: "anything"}); got.Note != "fallback" {
		t.Fatalf("fallback: %+v", got)
	}
	if calls := g.Calls(); len(calls) != 2 || calls[0].Text != "hello" {
		t.Fatalf("calls not recorded: %+v", calls)
	}

	failing := &FixtureGenerator{Err: errors.New("boom")}
	if _, err := failing.Draft(context.Background(), Input{}); err == nil {
		t.Fatal("error path not exercised")
	}
}

func TestSampleFixturePassesCheck(t *testing.T) {
	plan, err := SampleFixture().Draft(context.Background(), Input{Text: "吃素的不用分肉錢，小孩算半份"})
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := Check(plan, Input{Members: event().Members}); len(issues) != 0 {
		t.Fatalf("the shipped sample does not validate: %+v", issues)
	}
}

func TestPromptCarriesTheEventAndNotTheInstructions(t *testing.T) {
	// The rules a model most easily gets wrong must stay stated.
	for _, essential := range []string{"first match wins", "AND", "ratio, not a percentage", "0.1 to 100"} {
		if !strings.Contains(SystemPrompt(), essential) {
			t.Errorf("system prompt no longer states %q", essential)
		}
	}
	// Illustrative tags are fine; a real participant's name would mean event
	// data had been baked into instructions that are meant to be constant.
	if strings.Contains(SystemPrompt(), "小凱") {
		t.Error("system prompt contains member data")
	}

	msg, err := UserMessage(Input{Text: "吃素的不用分肉錢", ItemTags: []string{"肉品"}})
	if err != nil {
		t.Fatal(err)
	}
	var round promptContext
	if err := json.Unmarshal([]byte(msg), &round); err != nil {
		t.Fatalf("user message is not valid JSON: %v", err)
	}
	if round.Request != "吃素的不用分肉錢" {
		t.Errorf("request = %q", round.Request)
	}
	// Empty collections must render as [] so a model never sees null.
	empty, err := UserMessage(Input{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(empty, "null") {
		t.Errorf("null in prompt context: %s", empty)
	}
}

func TestVertexStaysOffUntilFullyConfigured(t *testing.T) {
	for _, c := range []Config{
		{},
		{Project: "p"},
		{Project: "p", Location: "asia-east1"},
		{Location: "asia-east1", Model: "m"},
	} {
		if c.Enabled() {
			t.Errorf("%+v reported enabled", c)
		}
		if _, err := NewVertexGenerator(t.Context(), c); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%+v: err = %v, want ErrNotConfigured", c, err)
		}
	}
	full := Config{Project: "p", Location: "asia-east1", Model: "m"}
	if !full.Enabled() {
		t.Fatal("complete config reported disabled")
	}
	if _, err := newVertexGenerator(t.Context(), full, &genai.ClientConfig{Credentials: fakeCredentials()}); err != nil {
		t.Fatalf("complete config refused: %v", err)
	}
}
