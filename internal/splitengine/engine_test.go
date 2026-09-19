package splitengine

import (
	"reflect"
	"testing"
)

func TestRules(t *testing.T) {
	r := []Rule{{Tag: "transport", Groups: []Group{{Conds: []string{"A", "B"}, Mode: "weight", Weight: 2}, {Conds: []string{"A"}, Mode: "weight", Weight: 0}}, Rest: &Group{Mode: "exclude"}}}
	cases := []struct {
		tag    string
		tags   []string
		weight float64
		kind   string
	}{{"transport", []string{"A", "B", "C"}, 2, "weighted"}, {"transport", []string{"A"}, 0, "excluded"}, {"transport", nil, 0, "excluded-rest"}, {"", nil, 1, "no-rule"}, {"unknown", nil, 1, "no-rule"}}
	for _, c := range cases {
		w, tr := ResolveWeight(c.tag, Member{ID: 1, Tags: c.tags}, r)
		if w != c.weight || tr.Kind != c.kind {
			t.Fatalf("%+v: %v %+v", c, w, tr)
		}
	}
}
func TestSplitAcceptance(t *testing.T) {
	ms := []Member{{ID: 3}, {ID: 2}, {ID: 1}}
	cases := []struct {
		name  string
		d     Detail
		rules []Rule
		want  []int64
		valid Validity
		diff  int64
	}{
		{"F3 order", Detail{Amount: 101}, nil, []int64{34, 34, 33}, OK, 0},
		{"custom fixed", Detail{Amount: 101, CustomShares: map[int64]int64{3: 20}}, nil, []int64{20, 41, 40}, OK, 0},
		{"manual subset", Detail{Amount: 101, ManualMemberIDs: []int64{1, 3}}, nil, []int64{51, 50}, OK, 0},
		{"empty manual", Detail{Amount: 101, ManualMemberIDs: []int64{}}, nil, []int64{}, NoParticipant, 0},
		{"overflow", Detail{Amount: 300, CustomShares: map[int64]int64{3: 350}}, nil, []int64{350, 0, 0}, CustomOverflow, 50},
		{"mismatch", Detail{Amount: 300, CustomShares: map[int64]int64{3: 100, 2: 100, 1: 50}}, nil, []int64{100, 100, 50}, CustomMismatch, 50},
		{"all fixed", Detail{Amount: 300, CustomShares: map[int64]int64{3: 100, 2: 100, 1: 100}}, nil, []int64{100, 100, 100}, OK, 0},
		{"rules ignore manual", Detail{Amount: 300, Tag: "t", ManualMemberIDs: []int64{}, CustomShares: map[int64]int64{3: 350}}, []Rule{{Tag: "t"}}, []int64{100, 100, 100}, OK, 0},
		{"rest excludes", Detail{Amount: 300, Tag: "t"}, []Rule{{Tag: "t", Rest: &Group{Mode: "exclude"}}}, []int64{}, NoParticipant, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SplitDetail(c.d, ms, c.rules, nil, 1)
			amounts := []int64{}
			for _, s := range got.Shares {
				amounts = append(amounts, s.Amount)
			}
			if !reflect.DeepEqual(amounts, c.want) || got.Validity != c.valid || got.Diff != c.diff {
				t.Fatalf("got %+v amounts %v", got, amounts)
			}
		})
	}
}
func TestOutdoor(t *testing.T) {
	ms := []Member{{1, []string{"adult", "driver"}}, {2, []string{"adult", "ride"}}, {3, []string{"adult", "veg", "self"}}, {4, []string{"child", "ride"}}}
	rules := []Rule{{Tag: "meat", Groups: []Group{{Conds: []string{"veg"}, Mode: "exclude"}, {Conds: []string{"child"}, Mode: "weight", Weight: .5}}}, {Tag: "transport", Groups: []Group{{Conds: []string{"self"}, Mode: "exclude"}, {Conds: []string{"ride"}, Mode: "weight", Weight: 1}}, Rest: &Group{Mode: "exclude"}}, {Tag: "alcohol", Groups: []Group{{Conds: []string{"driver"}, Mode: "exclude"}, {Conds: []string{"child"}, Mode: "exclude"}}}}
	for _, c := range []struct {
		tag    string
		amount int64
		want   map[int64]int64
	}{{"meat", 2800, map[int64]int64{1: 1120, 2: 1120, 4: 560}}, {"transport", 1200, map[int64]int64{2: 600, 4: 600}}, {"alcohol", 1100, map[int64]int64{2: 550, 3: 550}}, {"plates", 260, map[int64]int64{1: 65, 2: 65, 3: 65, 4: 65}}} {
		got := Compute(ms, []Item{{ID: 1, PayerID: 1, Details: []Detail{{Amount: c.amount, Tag: c.tag}}}}, rules)
		if !reflect.DeepEqual(got.PerDetail[0].Shares, c.want) {
			t.Fatalf("%s: %v", c.tag, got.PerDetail[0].Shares)
		}
	}
}
func TestHub(t *testing.T) {
	s := Shares{PerMember: map[int64]MemberShares{1: {Net: 0}, 2: {Net: 3000}, 3: {Net: -1500}, 4: {Net: -1500}}}
	got, err := HubTransfers(s, 1)
	want := []Transfer{{1, 2, 3000}, {3, 1, 1500}, {4, 1, 1500}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = HubTransfers(s, 99); err == nil {
		t.Fatal("missing host accepted")
	}
	s.PerMember[1] = MemberShares{Net: 1}
	if _, err = HubTransfers(s, 1); err == nil {
		t.Fatal("unbalanced accepted")
	}
}
func FuzzConservation(f *testing.F) {
	f.Add(uint32(101), uint8(3), uint8(5))
	f.Fuzz(func(t *testing.T, amount uint32, n, w uint8) {
		count := int(n%30) + 1
		ms := make([]Member, count)
		for i := range ms {
			ms[i] = Member{ID: int64(count - i)}
		}
		rule := Rule{Tag: "t", Rest: &Group{Mode: "weight", Weight: float64(w%100+1) / 10}}
		s := Compute(ms, []Item{{PayerID: ms[0].ID, Details: []Detail{{Amount: int64(amount), Tag: "t"}}}}, []Rule{rule})
		var owed, net int64
		for _, v := range s.PerMember {
			owed += v.Owed
			net += v.Net
		}
		if owed != int64(amount) || net != 0 {
			t.Fatal(s)
		}
		ts, err := HubTransfers(s, ms[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		var balance int64
		for _, tr := range ts {
			if tr.ToID == ms[0].ID {
				balance += tr.Amount
			}
			if tr.FromID == ms[0].ID {
				balance -= tr.Amount
			}
		}
		if balance != s.PerMember[ms[0].ID].Net {
			t.Fatal("hub sign")
		}
	})
}
