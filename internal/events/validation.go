package events

import (
	"context"
	"go-split-backend/internal/database"
	"go-split-backend/internal/splitengine"
	"strconv"
	"strings"
)

type detailIssue struct {
	ItemID   int64  `json:"item_id,omitempty"`
	DetailID int64  `json:"detail_id,omitempty"`
	Index    int    `json:"index"`
	Code     string `json:"code"`
	Diff     int64  `json:"diff,omitempty"`
}
type validationResponse struct {
	Error   string        `json:"error"`
	Details []detailIssue `json:"details"`
}

func splitIssues(r engineResult) []detailIssue {
	out := []detailIssue{}
	for i, d := range r.Shares.PerDetail {
		if d.Result.Validity != splitengine.OK {
			out = append(out, detailIssue{d.ItemID, d.DetailID, i, string(d.Result.Validity), d.Result.Diff})
		}
	}
	return out
}
func validateDetails(ctx context.Context, db database.Store, id, payerID int64, details []createDetailRequest) ([]detailIssue, error) {
	ms, err := loadEngineMembers(ctx, db, id)
	if err != nil {
		return nil, err
	}
	rules, err := loadEngineRules(ctx, db, id)
	if err != nil {
		return nil, err
	}
	tags, err := loadTagLabels(ctx, db, id, "event_item_tags")
	if err != nil {
		return nil, err
	}
	out := []detailIssue{}
	for i, d := range details {
		code, tag := validateDetailInput(d, ms, rules, tags)
		if code != "" {
			out = append(out, detailIssue{Index: i, Code: code})
			continue
		}
		r := splitengine.SplitDetail(splitengine.Detail{Amount: d.Amount, Tag: tag, ManualMemberIDs: d.ManualMemberIDs, CustomShares: numericStringMap(d.CustomShares), PayerID: payerID}, ms, rules, nil, 1)
		if r.Validity != splitengine.OK {
			out = append(out, detailIssue{Index: i, Code: string(r.Validity), Diff: r.Diff})
		}
	}
	return out, nil
}
func validateDetailInput(d createDetailRequest, ms []splitengine.Member, rules []splitengine.Rule, tags []string) (string, string) {
	code := ""
	tag := ""
	if d.Tag != nil {
		tag = *d.Tag
	}
	switch {
	case strings.TrimSpace(d.Name) == "" || len(d.Name) > 120:
		code = "invalid-name"
	case d.Amount < 0 || d.Amount > 1_000_000_000_000:
		code = "invalid-amount"
	case tag != "" && !stringIn(tags, tag):
		code = "unknown-item-tag"
	}
	ruled := false
	for _, r := range rules {
		if r.Tag == tag {
			ruled = true
		}
	}
	if ruled && (d.ManualMemberIDs != nil || len(d.CustomShares) > 0) {
		code = "rule-lock"
	}
	pool := map[int64]bool{}
	for _, m := range ms {
		pool[m.ID] = true
	}
	manual := map[int64]bool{}
	for _, mid := range d.ManualMemberIDs {
		if !pool[mid] || manual[mid] {
			code = "invalid-participant"
		}
		manual[mid] = true
	}
	for key, amount := range d.CustomShares {
		mid, e := strconv.ParseInt(key, 10, 64)
		if e != nil || strconv.FormatInt(mid, 10) != key || !pool[mid] || amount < 0 || amount > 1_000_000_000_000 || (d.ManualMemberIDs != nil && !manual[mid]) {
			code = "invalid-custom-amount"
		}
	}
	return code, tag
}
func stringIn(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
