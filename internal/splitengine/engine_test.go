package splitengine

import (
	"reflect"
	"testing"
)

func mk(id int64, tags ...string) Member {
	return Member{ID: id, Tags: tags}
}

func detail(id, amount int64, tag string) Detail {
	return Detail{ID: id, AmountCents: amount, Tag: tag}
}

// PRD F2 golden case: 1300 元, A-tag members weight 2, B-tag members weight 3.
// Expected per-person: A each 200, B each 300.
func TestCompute_PRDGoldenExample(t *testing.T) {
	members := []Member{
		mk(1, "A"), mk(2, "A"),
		mk(3, "B"), mk(4, "B"), mk(5, "B"),
	}
	rule := Rule{
		Tag: "T",
		Groups: []Group{
			{Conds: []string{"A"}, Mode: "weight", Weight: 2},
			{Conds: []string{"B"}, Mode: "weight", Weight: 3},
		},
	}
	items := []Item{{ID: 100, PayerID: 1, Details: []Detail{detail(1000, 1300, "T")}}}

	got := Compute(members, items, []Rule{rule})

	want := map[int64]int64{1: 200, 2: 200, 3: 300, 4: 300, 5: 300}
	for id, w := range want {
		if s := got.PerDetail[0].Shares[id]; s != w {
			t.Errorf("member %d: got %d, want %d", id, s, w)
		}
	}
	if got.GrandTotalCents != 1300 {
		t.Errorf("grand total: got %d, want 1300", got.GrandTotalCents)
	}
}

// PRD F3 mechanism 1: 101 with three equal-weight members. Floors 33 each;
// leftover 2 goes to the first two members in id order.
func TestCompute_RoundingRemainderInIDOrder(t *testing.T) {
	members := []Member{mk(1), mk(2), mk(3)}
	items := []Item{{ID: 1, PayerID: 1, Details: []Detail{detail(1, 101, "")}}}
	got := Compute(members, items, nil)
	want := map[int64]int64{1: 34, 2: 34, 3: 33}
	if !reflect.DeepEqual(got.PerDetail[0].Shares, want) {
		t.Errorf("shares: got %v, want %v", got.PerDetail[0].Shares, want)
	}
}

// PRD F3 mechanism 2: 300 with A=150 custom, B and C split the remainder equally.
func TestCompute_CustomShareOverride(t *testing.T) {
	members := []Member{mk(1), mk(2), mk(3)}
	items := []Item{{ID: 1, PayerID: 1, Details: []Detail{{
		ID: 1, AmountCents: 300, CustomShares: map[int64]int64{1: 150},
	}}}}
	got := Compute(members, items, nil)
	want := map[int64]int64{1: 150, 2: 75, 3: 75}
	if !reflect.DeepEqual(got.PerDetail[0].Shares, want) {
		t.Errorf("shares: got %v, want %v", got.PerDetail[0].Shares, want)
	}
}

// PRD F2 divide-by-zero guard: everyone excluded → all shares 0, no panic.
func TestCompute_AllExcluded(t *testing.T) {
	members := []Member{mk(1, "veg"), mk(2, "veg")}
	rule := Rule{Tag: "meat", Groups: []Group{{Conds: []string{"veg"}, Mode: "exclude"}}}
	items := []Item{{ID: 1, PayerID: 1, Details: []Detail{detail(1, 1000, "meat")}}}
	got := Compute(members, items, []Rule{rule})
	for id, cents := range got.PerDetail[0].Shares {
		if cents != 0 {
			t.Errorf("member %d: got %d, want 0", id, cents)
		}
	}
}

// A rule's `rest` weight applies when no group matched.
func TestCompute_RestWeightApplies(t *testing.T) {
	members := []Member{mk(1, "vip"), mk(2), mk(3)}
	rest := Group{Mode: "weight", Weight: 1}
	rule := Rule{
		Tag:    "t",
		Groups: []Group{{Conds: []string{"vip"}, Mode: "weight", Weight: 2}},
		Rest:   &rest,
	}
	items := []Item{{ID: 1, PayerID: 1, Details: []Detail{detail(1, 400, "t")}}}
	got := Compute(members, items, []Rule{rule})
	// total weight = 2 + 1 + 1 = 4, per unit = 100
	want := map[int64]int64{1: 200, 2: 100, 3: 100}
	if !reflect.DeepEqual(got.PerDetail[0].Shares, want) {
		t.Errorf("shares: got %v, want %v", got.PerDetail[0].Shares, want)
	}
}

// A `rest: exclude` on the transport rule leaves only members who match a
// group, mirroring PRD's outdoor template rule for 交通費.
func TestCompute_RestExcludeKeepsOnlyMatched(t *testing.T) {
	members := []Member{mk(1, "car"), mk(2)} // 1 needs a ride, 2 goes on their own
	restEx := Group{Mode: "exclude"}
	rule := Rule{
		Tag:    "transport",
		Groups: []Group{{Conds: []string{"car"}, Mode: "weight", Weight: 1}},
		Rest:   &restEx,
	}
	items := []Item{{ID: 1, PayerID: 1, Details: []Detail{detail(1, 500, "transport")}}}
	got := Compute(members, items, []Rule{rule})
	want := map[int64]int64{1: 500, 2: 0}
	if !reflect.DeepEqual(got.PerDetail[0].Shares, want) {
		t.Errorf("shares: got %v, want %v", got.PerDetail[0].Shares, want)
	}
}

// Two-detail example combines owed and paid across members.
func TestCompute_PerMemberNetAcrossDetails(t *testing.T) {
	members := []Member{mk(1), mk(2), mk(3)}
	// 300 paid by 1, 300 paid by 2, no rules → 100 each per detail.
	items := []Item{
		{ID: 1, PayerID: 1, Details: []Detail{detail(1, 300, "")}},
		{ID: 2, PayerID: 2, Details: []Detail{detail(2, 300, "")}},
	}
	got := Compute(members, items, nil)
	if got.PerMember[1].NetCents != 100 { // paid 300, owed 200 (100 + 100)
		t.Errorf("member 1 net: got %d, want 100", got.PerMember[1].NetCents)
	}
	if got.PerMember[2].NetCents != 100 {
		t.Errorf("member 2 net: got %d, want 100", got.PerMember[2].NetCents)
	}
	if got.PerMember[3].NetCents != -200 { // paid 0, owed 200
		t.Errorf("member 3 net: got %d, want -200", got.PerMember[3].NetCents)
	}
}

func TestTransfers_GreedyLargestFirst(t *testing.T) {
	s := Shares{PerMember: map[int64]MemberShares{
		1: {NetCents: 200},
		2: {NetCents: 200},
		3: {NetCents: -300},
		4: {NetCents: -100},
	}}
	got := Transfers(s)
	// Largest debtor 3 (300) meets largest creditor 1 (200) → transfer 200,
	// then remaining 100 goes to creditor 2. Debtor 4 (100) meets remainder
	// of creditor 2 (100) → 100.
	want := []Transfer{
		{FromID: 3, ToID: 1, AmountCents: 200},
		{FromID: 3, ToID: 2, AmountCents: 100},
		{FromID: 4, ToID: 2, AmountCents: 100},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHubTransfers_EveryoneThroughHub(t *testing.T) {
	s := Shares{PerMember: map[int64]MemberShares{
		1: {NetCents: 300},  // hub owes 1
		2: {NetCents: -100}, // 2 pays hub
		3: {NetCents: -200}, // 3 pays hub
		9: {NetCents: 0},    // hub
	}}
	got := HubTransfers(s, 9)
	want := []Transfer{
		{FromID: 9, ToID: 1, AmountCents: 300},
		{FromID: 2, ToID: 9, AmountCents: 100},
		{FromID: 3, ToID: 9, AmountCents: 200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPairBreakdown_OnlyDetailsInvolvingBothMembers(t *testing.T) {
	members := []Member{mk(1), mk(2), mk(3)}
	items := []Item{
		// paid by 1: 300 split three ways → 2 owes 1 = 100
		{ID: 1, PayerID: 1, Details: []Detail{detail(1, 300, "")}},
		// paid by 2: 300 split three ways → 1 owes 2 = 100
		{ID: 2, PayerID: 2, Details: []Detail{detail(2, 300, "")}},
		// paid by 3: no direct debt between 1 and 2
		{ID: 3, PayerID: 3, Details: []Detail{detail(3, 300, "")}},
	}
	s := Compute(members, items, nil)
	got := PairBreakdown(s, items, 1, 2)
	want := []PairLine{
		{ItemID: 1, DetailID: 1, FromID: 2, ToID: 1, AmountCents: 100},
		{ItemID: 2, DetailID: 2, FromID: 1, ToID: 2, AmountCents: 100},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
