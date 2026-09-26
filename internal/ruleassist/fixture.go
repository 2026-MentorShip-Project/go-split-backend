package ruleassist

import (
	"context"
	"encoding/json"
	"sync"
)

// FixtureGenerator answers from canned plans. It exists so tests and local
// development exercise the whole draft path — validation, preview, apply —
// without a model call, a network dependency or a bill.
type FixtureGenerator struct {
	// Plans answers requests whose Text matches a key exactly.
	Plans map[string]Plan
	// Fallback answers anything unlisted. Nil yields an empty plan.
	Fallback *Plan
	// Err, when set, fails every call, for exercising the error path.
	Err error

	mu    sync.Mutex
	calls []Input
}

// Draft implements Generator.
func (g *FixtureGenerator) Draft(_ context.Context, in Input) (Plan, error) {
	g.mu.Lock()
	g.calls = append(g.calls, in)
	g.mu.Unlock()

	if g.Err != nil {
		return Plan{}, g.Err
	}
	if plan, ok := g.Plans[in.Text]; ok {
		return plan, nil
	}
	if g.Fallback != nil {
		return *g.Fallback, nil
	}
	return Plan{Note: "no fixture for this request"}, nil
}

// Calls returns the requests received so far, for asserting what a handler sent.
func (g *FixtureGenerator) Calls() []Input {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Input(nil), g.calls...)
}

// SampleFixture is a generator seeded with one realistic plan, so a developer
// can drive the feature end to end before any model is configured. The request
// it answers is the worked example from the barbecue template.
func SampleFixture() *FixtureGenerator {
	plan := Plan{
		NewItemTags: []string{"肉品"},
		NewCondTags: []string{"吃素", "小孩"},
		Rules: []PlannedRule{{
			Op:      Create,
			ItemTag: "肉品",
			Groups: json.RawMessage(`[
				{"conds":["吃素"],"mode":"exclude"},
				{"conds":["小孩"],"mode":"weight","weight":0.5}
			]`),
			Rest: json.RawMessage(`{"mode":"weight","weight":1}`),
			Note: "吃素的人不分攤肉錢，小孩算半份，其他人各一份",
		}},
		MemberConds: []MemberCond{},
		Note:        "依你的描述建立了「肉品」的分攤規則",
	}
	return &FixtureGenerator{
		Plans:    map[string]Plan{"吃素的不用分肉錢，小孩算半份": plan},
		Fallback: &plan,
	}
}
