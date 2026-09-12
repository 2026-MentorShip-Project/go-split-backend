// Package splitengine turns members, expense details, and per-event rules
// into per-detail shares, per-member totals, and settlement transfer lists.
// Pure Go, no I/O: hand it typed slices and it returns typed structs.
package splitengine

import (
	"math"
	"sort"
)

type Member struct {
	ID   int64
	Tags []string
}

type Group struct {
	Conds  []string
	Mode   string
	Weight float64
}

type Rule struct {
	Tag    string
	Groups []Group
	Rest   *Group
}

type Detail struct {
	ID           int64
	AmountCents  int64
	Tag          string
	CustomShares map[int64]int64
}

type Item struct {
	ID      int64
	PayerID int64
	Details []Detail
}

type DetailShares struct {
	ItemID      int64
	DetailID    int64
	AmountCents int64
	Shares      map[int64]int64
}

type MemberShares struct {
	OwedCents int64
	PaidCents int64
	NetCents  int64
}

type Shares struct {
	GrandTotalCents int64
	PerDetail       []DetailShares
	PerMember       map[int64]MemberShares
}

type Transfer struct {
	FromID      int64
	ToID        int64
	AmountCents int64
}

type PairLine struct {
	ItemID      int64
	DetailID    int64
	FromID      int64
	ToID        int64
	AmountCents int64
}

// Compute returns the whole share breakdown in one pass. Members not
// referenced by any item still appear in PerMember with zero totals.
func Compute(members []Member, items []Item, rules []Rule) Shares {
	ruleByTag := map[string]Rule{}
	for _, r := range rules {
		ruleByTag[r.Tag] = r
	}
	orderedIDs := sortedMemberIDs(members)

	perMember := map[int64]*MemberShares{}
	for _, m := range members {
		perMember[m.ID] = &MemberShares{}
	}

	perDetail := []DetailShares{}
	var grand int64
	for _, it := range items {
		for _, d := range it.Details {
			shares := splitDetail(members, orderedIDs, d, ruleByTag[d.Tag])
			perDetail = append(perDetail, DetailShares{
				ItemID:      it.ID,
				DetailID:    d.ID,
				AmountCents: d.AmountCents,
				Shares:      shares,
			})
			for id, cents := range shares {
				if ms, ok := perMember[id]; ok {
					ms.OwedCents += cents
				}
			}
			grand += d.AmountCents
			if pm, ok := perMember[it.PayerID]; ok {
				pm.PaidCents += d.AmountCents
			}
		}
	}
	out := Shares{
		GrandTotalCents: grand,
		PerDetail:       perDetail,
		PerMember:       map[int64]MemberShares{},
	}
	for id, ms := range perMember {
		ms.NetCents = ms.PaidCents - ms.OwedCents
		out.PerMember[id] = *ms
	}
	return out
}

func splitDetail(members []Member, orderedIDs []int64, d Detail, rule Rule) map[int64]int64 {
	shares := map[int64]int64{}
	weights := map[int64]float64{}
	customTotal := int64(0)

	for _, m := range members {
		if v, ok := d.CustomShares[m.ID]; ok {
			shares[m.ID] = v
			customTotal += v
			continue
		}
		weights[m.ID] = weightFor(m, rule)
	}

	remainder := d.AmountCents - customTotal
	if remainder < 0 {
		remainder = 0
	}

	var totalWeight float64
	for _, w := range weights {
		if w > 0 {
			totalWeight += w
		}
	}
	if totalWeight == 0 {
		for id := range weights {
			shares[id] = 0
		}
		return shares
	}

	floors := map[int64]int64{}
	var floorSum int64
	for id, w := range weights {
		if w <= 0 {
			shares[id] = 0
			continue
		}
		exact := float64(remainder) * w / totalWeight
		f := int64(math.Floor(exact))
		floors[id] = f
		floorSum += f
	}
	leftover := remainder - floorSum

	eligibleOrder := make([]int64, 0, len(floors))
	for _, id := range orderedIDs {
		if _, ok := floors[id]; ok {
			eligibleOrder = append(eligibleOrder, id)
		}
	}
	for i := int64(0); i < leftover && len(eligibleOrder) > 0; i++ {
		floors[eligibleOrder[int(i)%len(eligibleOrder)]]++
	}
	for id, cents := range floors {
		shares[id] = cents
	}
	return shares
}

func weightFor(m Member, r Rule) float64 {
	for _, g := range r.Groups {
		if !allTagsPresent(m.Tags, g.Conds) {
			continue
		}
		if g.Mode == "exclude" {
			return 0
		}
		if g.Weight <= 0 {
			return 1
		}
		return g.Weight
	}
	if r.Rest == nil {
		return 1
	}
	if r.Rest.Mode == "exclude" {
		return 0
	}
	if r.Rest.Weight <= 0 {
		return 1
	}
	return r.Rest.Weight
}

func allTagsPresent(memberTags, groupConds []string) bool {
	if len(groupConds) == 0 {
		return false
	}
	for _, c := range groupConds {
		found := false
		for _, mt := range memberTags {
			if mt == c {
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

func sortedMemberIDs(members []Member) []int64 {
	ids := make([]int64, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Transfers greedy-matches debtors to creditors, largest first. Returns at
// most N-1 transfers for N members.
func Transfers(s Shares) []Transfer {
	type entry struct {
		id  int64
		val int64
	}
	ids := sortedMemberIDsFromMap(s.PerMember)

	var debtors, creditors []entry
	for _, id := range ids {
		net := s.PerMember[id].NetCents
		switch {
		case net < 0:
			debtors = append(debtors, entry{id: id, val: -net})
		case net > 0:
			creditors = append(creditors, entry{id: id, val: net})
		}
	}
	sort.SliceStable(debtors, func(i, j int) bool { return debtors[i].val > debtors[j].val })
	sort.SliceStable(creditors, func(i, j int) bool { return creditors[i].val > creditors[j].val })

	transfers := []Transfer{}
	di, ci := 0, 0
	for di < len(debtors) && ci < len(creditors) {
		amt := debtors[di].val
		if creditors[ci].val < amt {
			amt = creditors[ci].val
		}
		transfers = append(transfers, Transfer{
			FromID:      debtors[di].id,
			ToID:        creditors[ci].id,
			AmountCents: amt,
		})
		debtors[di].val -= amt
		creditors[ci].val -= amt
		if debtors[di].val == 0 {
			di++
		}
		if creditors[ci].val == 0 {
			ci++
		}
	}
	return transfers
}

// HubTransfers settles everyone through hubID. Positive-net members receive
// from hub; negative-net members pay hub.
func HubTransfers(s Shares, hubID int64) []Transfer {
	transfers := []Transfer{}
	for _, id := range sortedMemberIDsFromMap(s.PerMember) {
		if id == hubID {
			continue
		}
		net := s.PerMember[id].NetCents
		switch {
		case net > 0:
			transfers = append(transfers, Transfer{FromID: hubID, ToID: id, AmountCents: net})
		case net < 0:
			transfers = append(transfers, Transfer{FromID: id, ToID: hubID, AmountCents: -net})
		}
	}
	return transfers
}

// PairBreakdown lists every detail that contributes to a direct debt
// between aID and bID. A detail contributes only when one of the two paid
// it and the other owes a nonzero share.
func PairBreakdown(s Shares, items []Item, aID, bID int64) []PairLine {
	byDetail := map[int64]DetailShares{}
	for _, ds := range s.PerDetail {
		byDetail[ds.DetailID] = ds
	}
	lines := []PairLine{}
	for _, it := range items {
		for _, d := range it.Details {
			ds, ok := byDetail[d.ID]
			if !ok {
				continue
			}
			switch it.PayerID {
			case aID:
				if v := ds.Shares[bID]; v != 0 {
					lines = append(lines, PairLine{
						ItemID: it.ID, DetailID: d.ID,
						FromID: bID, ToID: aID, AmountCents: v,
					})
				}
			case bID:
				if v := ds.Shares[aID]; v != 0 {
					lines = append(lines, PairLine{
						ItemID: it.ID, DetailID: d.ID,
						FromID: aID, ToID: bID, AmountCents: v,
					})
				}
			}
		}
	}
	return lines
}

func sortedMemberIDsFromMap(m map[int64]MemberShares) []int64 {
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
