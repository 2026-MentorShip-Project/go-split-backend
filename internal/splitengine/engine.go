// Package splitengine provides the pure, shared R1 splitting engine.
package splitengine

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
)

const Version = "1.3.0"

type Validity string

const (
	OK             Validity = "ok"
	NoParticipant  Validity = "no-participant"
	CustomMismatch Validity = "custom-mismatch"
	CustomOverflow Validity = "custom-overflow"
)

type Member struct {
	ID   int64    `json:"id"`
	Tags []string `json:"cond_tags"`
}
type Group struct {
	Conds  []string `json:"conds,omitempty"`
	Mode   string   `json:"mode"`
	Weight float64  `json:"weight"`
}

// UnmarshalJSON preserves explicit zero while defaulting omitted weights to one.
func (g *Group) UnmarshalJSON(data []byte) error {
	type plain Group
	value := plain{Weight: 1}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*g = Group(value)
	return nil
}

type Rule struct {
	Tag    string  `json:"item_tag"`
	Groups []Group `json:"groups"`
	Rest   *Group  `json:"rest,omitempty"`
}
type Detail struct {
	ID              int64           `json:"id"`
	Amount          int64           `json:"amount"`
	Tag             string          `json:"item_tag"`
	ManualMemberIDs []int64         `json:"manual_member_ids"`
	CustomShares    map[int64]int64 `json:"custom_amounts"`
	// PayerID absorbs the detail when no member shares it.
	PayerID int64 `json:"payer_id,omitempty"`
}
type Item struct {
	ID      int64    `json:"id"`
	PayerID int64    `json:"payer_id"`
	Details []Detail `json:"details"`
}
type Trace struct {
	Kind           string   `json:"kind"`
	RuleItemTag    string   `json:"rule_item_tag,omitempty"`
	CondSetIndex   *int     `json:"cond_set_index,omitempty"`
	HitCondTags    []string `json:"hit_cond_tags,omitempty"`
	Weight         float64  `json:"weight,omitempty"`
	TotalWeight    float64  `json:"total_weight,omitempty"`
	UnitPrice      float64  `json:"unit_price,omitempty"`
	RemainderBonus int64    `json:"remainder_bonus"`
	Value          int64    `json:"value,omitempty"`
}
type Share struct {
	MemberID int64 `json:"member_id"`
	Amount   int64 `json:"amount"`
	Trace    Trace `json:"trace"`
}
type SplitResult struct {
	Shares      []Share  `json:"shares"`
	Excluded    []Share  `json:"excluded"`
	TotalWeight float64  `json:"total_weight"`
	UnitPrice   float64  `json:"unit_price"`
	Validity    Validity `json:"validity"`
	Diff        int64    `json:"diff,omitempty"`
}
type DetailShares struct {
	ItemID   int64
	DetailID int64
	Amount   int64
	Shares   map[int64]int64
	Result   SplitResult
}
type MemberShares struct {
	Owed int64
	Paid int64
	Net  int64
}
type Shares struct {
	GrandTotal int64
	PerDetail  []DetailShares
	PerMember  map[int64]MemberShares
}
type Transfer struct {
	FromID int64 `json:"from_id"`
	ToID   int64 `json:"to_id"`
	Amount int64 `json:"amount"`
}

// ResolveWeight distinguishes unmatched rules from absent rules.
func ResolveWeight(tag string, member Member, rules []Rule) (float64, Trace) {
	for _, r := range rules {
		if tag == "" || r.Tag != tag {
			continue
		}
		for i, g := range r.Groups {
			if !allTagsPresent(member.Tags, g.Conds) {
				continue
			}
			trace := Trace{Kind: "weighted", RuleItemTag: tag, CondSetIndex: &i, HitCondTags: append([]string(nil), g.Conds...), Weight: g.Weight}
			if g.Mode == "exclude" || g.Weight == 0 {
				trace.Kind = "excluded"
				trace.Weight = 0
				return 0, trace
			}
			return g.Weight, trace
		}
		w := 1.0
		if r.Rest != nil {
			if r.Rest.Mode == "exclude" || r.Rest.Weight == 0 {
				return 0, Trace{Kind: "excluded-rest", RuleItemTag: tag}
			}
			w = r.Rest.Weight
		}
		return w, Trace{Kind: "rest", RuleItemTag: tag, Weight: w}
	}
	return 1, Trace{Kind: "no-rule", Weight: 1}
}
func allTagsPresent(tags, conds []string) bool {
	for _, c := range conds {
		found := false
		for _, v := range tags {
			if v == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// SplitDetail assumes validated inputs. Invalid business splits are returned as
// validity values, never repaired by altering fixed custom amounts.
func SplitDetail(d Detail, members []Member, rules []Rule, order []int64, minUnit int64) SplitResult {
	out := SplitResult{Shares: []Share{}, Excluded: []Share{}, Validity: OK}
	ruled := false
	for _, r := range rules {
		if d.Tag != "" && r.Tag == d.Tag {
			ruled = true
			break
		}
	}
	weights := map[int64]int64{}
	fixed := map[int64]bool{}
	var fixedSum, totalWeight int64
	for _, m := range members {
		if !ruled && d.ManualMemberIDs != nil && !contains(d.ManualMemberIDs, m.ID) {
			continue
		}
		w, tr := ResolveWeight(d.Tag, m, rules)
		if w <= 0 {
			out.Excluded = append(out.Excluded, Share{MemberID: m.ID, Trace: tr})
			continue
		}
		sh := Share{MemberID: m.ID, Trace: tr}
		if v, ok := d.CustomShares[m.ID]; ok && !ruled {
			sh.Amount = v
			sh.Trace = Trace{Kind: "custom", Value: v}
			fixed[m.ID] = true
			fixedSum += v
		} else {
			weights[m.ID] = int64(math.Round(w * 10))
			totalWeight += weights[m.ID]
		}
		out.Shares = append(out.Shares, sh)
	}
	if len(out.Shares) == 0 {
		return payerAbsorbs(out, d, members)
	}
	out.TotalWeight = float64(totalWeight) / 10
	switch {
	case fixedSum > d.Amount:
		out.Validity = CustomOverflow
		out.Diff = fixedSum - d.Amount
	case totalWeight == 0 && fixedSum != d.Amount:
		out.Validity = CustomMismatch
		out.Diff = d.Amount - fixedSum
	}
	if totalWeight == 0 || out.Validity != OK {
		return out
	}
	remaining := d.Amount - fixedSum
	out.UnitPrice = float64(remaining) / out.TotalWeight
	if minUnit <= 0 {
		minUnit = 1
	}
	settleAllocations(&out, weights, fixed, remaining, totalWeight, order, minUnit)
	return out
}
func settleAllocations(out *SplitResult, weights map[int64]int64, fixed map[int64]bool, remaining, totalWeight int64, order []int64, minUnit int64) {
	var allocated int64
	index := map[int64]int{}
	for i := range out.Shares {
		sh := &out.Shares[i]
		if fixed[sh.MemberID] {
			continue
		}
		// Integer rational arithmetic prevents float-dependent remainder recipients.
		numerator := new(big.Int).Mul(big.NewInt(remaining), big.NewInt(weights[sh.MemberID]))
		denominator := new(big.Int).Mul(big.NewInt(totalWeight), big.NewInt(minUnit))
		sh.Amount = new(big.Int).Quo(numerator, denominator).Int64() * minUnit
		sh.Trace.TotalWeight = out.TotalWeight
		sh.Trace.UnitPrice = out.UnitPrice
		allocated += sh.Amount
		index[sh.MemberID] = i
	}
	if order == nil {
		for _, sh := range out.Shares {
			order = append(order, sh.MemberID)
		}
	}
	// Ignore stale/duplicate IDs and retain all eligible members deterministically.
	eligible := []int64{}
	for _, id := range order {
		if _, ok := index[id]; ok && !contains(eligible, id) {
			eligible = append(eligible, id)
		}
	}
	for _, sh := range out.Shares {
		if !fixed[sh.MemberID] && !contains(eligible, sh.MemberID) {
			eligible = append(eligible, sh.MemberID)
		}
	}
	left := remaining - allocated
	for i := 0; left >= minUnit && len(eligible) > 0; i++ {
		sh := &out.Shares[index[eligible[i%len(eligible)]]]
		sh.Amount += minUnit
		sh.Trace.RemainderBonus += minUnit
		left -= minUnit
	}
}

// payerAbsorbs charges the whole detail to its payer when no member shares
// it, so the line nets to zero. NoParticipant remains only without a payer.
func payerAbsorbs(out SplitResult, d Detail, members []Member) SplitResult {
	for _, m := range members {
		if m.ID != d.PayerID {
			continue
		}
		excluded := []Share{}
		for _, sh := range out.Excluded {
			if sh.MemberID != m.ID {
				excluded = append(excluded, sh)
			}
		}
		out.Excluded = excluded
		out.Shares = []Share{{MemberID: m.ID, Amount: d.Amount, Trace: Trace{Kind: "payer-absorbs", Value: d.Amount}}}
		return out
	}
	out.Validity = NoParticipant
	return out
}
func Compute(members []Member, items []Item, rules []Rule) Shares {
	out := Shares{PerDetail: []DetailShares{}, PerMember: map[int64]MemberShares{}}
	for _, m := range members {
		out.PerMember[m.ID] = MemberShares{}
	}
	for _, it := range items {
		for _, d := range it.Details {
			d.PayerID = it.PayerID
			r := SplitDetail(d, members, rules, nil, 1)
			ds := DetailShares{ItemID: it.ID, DetailID: d.ID, Amount: d.Amount, Shares: map[int64]int64{}, Result: r}
			for _, sh := range r.Shares {
				ds.Shares[sh.MemberID] = sh.Amount
				ms := out.PerMember[sh.MemberID]
				ms.Owed += sh.Amount
				out.PerMember[sh.MemberID] = ms
			}
			out.PerDetail = append(out.PerDetail, ds)
			out.GrandTotal += d.Amount
			ms := out.PerMember[it.PayerID]
			ms.Paid += d.Amount
			out.PerMember[it.PayerID] = ms
		}
	}
	for id, ms := range out.PerMember {
		ms.Net = ms.Paid - ms.Owed
		out.PerMember[id] = ms
	}
	return out
}

// HubTransfers requires a balanced set and an existing host.
func HubTransfers(s Shares, hubID int64) ([]Transfer, error) {
	if _, ok := s.PerMember[hubID]; !ok {
		return nil, errors.New("host is not a member")
	}
	var sum int64
	ids := []int64{}
	for id, ms := range s.PerMember {
		sum += ms.Net
		ids = append(ids, id)
	}
	if sum != 0 {
		return nil, errors.New("unbalanced settlement")
	}
	// Transfer ordering is independent of remainder ordering.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	out := []Transfer{}
	for _, id := range ids {
		if id == hubID {
			continue
		}
		n := s.PerMember[id].Net
		if n > 0 {
			out = append(out, Transfer{hubID, id, n})
		} else if n < 0 {
			out = append(out, Transfer{id, hubID, -n})
		}
	}
	return out, nil
}
