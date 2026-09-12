package events

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
	"go-split-backend/internal/splitengine"
)

func (h *Handler) registerSharesRoutes(g *gin.RouterGroup) {
	anyRole := auth.RequireEventRole(h.DB, "host", "co", "member")
	g.GET("/:id/shares", anyRole, h.GetShares)
	g.GET("/:id/transfers", anyRole, h.GetTransfers)
	g.GET("/:id/pairs/:a/:b", anyRole, h.GetPair)
}

type memberShareDTO struct {
	MemberID  int64 `json:"member_id"`
	OwedCents int64 `json:"owed_cents"`
	PaidCents int64 `json:"paid_cents"`
	NetCents  int64 `json:"net_cents"`
}

type detailShareDTO struct {
	ItemID      int64            `json:"item_id"`
	DetailID    int64            `json:"detail_id"`
	AmountCents int64            `json:"amount_cents"`
	Shares      map[string]int64 `json:"shares"`
}

type sharesResponse struct {
	GrandTotalCents int64            `json:"grand_total_cents"`
	PerMember       []memberShareDTO `json:"per_member"`
	PerDetail       []detailShareDTO `json:"per_detail"`
}

type transferDTO struct {
	FromID      int64 `json:"from_id"`
	ToID        int64 `json:"to_id"`
	AmountCents int64 `json:"amount_cents"`
}

type transfersResponse struct {
	Mode      string        `json:"mode"`
	HubID     int64         `json:"hub_id,omitempty"`
	Transfers []transferDTO `json:"transfers"`
}

// GetShares godoc
// @Summary     Compute per-member and per-detail shares for an event
// @Description Any member may call. Runs the split engine against the
// @Description event's members, items, and rules and returns the whole
// @Description breakdown. Amounts are in cents.
// @Tags        shares
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} sharesResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/shares [get]
func (h *Handler) GetShares(c *gin.Context) {
	eventID := eventIDFromPath(c)
	ctx := c.Request.Context()

	result, err := computeEventShares(ctx, h.DB, eventID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "compute shares")
		return
	}
	c.JSON(http.StatusOK, sharesFromResult(result))
}

// GetTransfers godoc
// @Summary     List settlement transfers for an event
// @Description Any member may call. mode=min (default) greedy-matches
// @Description debtors to creditors. mode=hub routes every non-zero net
// @Description through hub (a member id passed as ?hub=).
// @Tags        shares
// @Produce     json
// @Param       id   path  int    true  "Event id"
// @Param       mode query string false "min or hub"
// @Param       hub  query int    false "member id when mode=hub"
// @Success     200  {object} transfersResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Router      /events/{id}/transfers [get]
func (h *Handler) GetTransfers(c *gin.Context) {
	eventID := eventIDFromPath(c)
	mode := c.DefaultQuery("mode", "min")
	if mode != "min" && mode != "hub" {
		respondErr(c, http.StatusBadRequest, "mode must be min or hub")
		return
	}
	var hubID int64
	if mode == "hub" {
		v, err := strconv.ParseInt(c.Query("hub"), 10, 64)
		if err != nil || v <= 0 {
			respondErr(c, http.StatusBadRequest, "hub is required when mode=hub")
			return
		}
		hubID = v
	}

	result, err := computeEventShares(c.Request.Context(), h.DB, eventID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "compute shares")
		return
	}

	var raw []splitengine.Transfer
	if mode == "hub" {
		raw = splitengine.HubTransfers(result.Shares, hubID)
	} else {
		raw = splitengine.Transfers(result.Shares)
	}
	out := transfersResponse{Mode: mode, HubID: hubID, Transfers: make([]transferDTO, 0, len(raw))}
	for _, t := range raw {
		out.Transfers = append(out.Transfers, transferDTO{FromID: t.FromID, ToID: t.ToID, AmountCents: t.AmountCents})
	}
	c.JSON(http.StatusOK, out)
}

type engineResult struct {
	Members []splitengine.Member
	Items   []splitengine.Item
	Rules   []splitengine.Rule
	Shares  splitengine.Shares
}

func computeEventShares(ctx context.Context, db *pgxpool.Pool, eventID int64) (engineResult, error) {
	members, err := loadEngineMembers(ctx, db, eventID)
	if err != nil {
		return engineResult{}, err
	}
	items, err := loadEngineItems(ctx, db, eventID)
	if err != nil {
		return engineResult{}, err
	}
	rules, err := loadEngineRules(ctx, db, eventID)
	if err != nil {
		return engineResult{}, err
	}
	return engineResult{
		Members: members,
		Items:   items,
		Rules:   rules,
		Shares:  splitengine.Compute(members, items, rules),
	}, nil
}

func loadEngineMembers(ctx context.Context, db *pgxpool.Pool, eventID int64) ([]splitengine.Member, error) {
	rows, err := db.Query(ctx,
		`SELECT id, tags FROM event_members WHERE event_id = $1 ORDER BY id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []splitengine.Member{}
	for rows.Next() {
		var m splitengine.Member
		if err := rows.Scan(&m.ID, &m.Tags); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadEngineItems(ctx context.Context, db *pgxpool.Pool, eventID int64) ([]splitengine.Item, error) {
	rows, err := db.Query(ctx, `
		SELECT i.id, i.payer_member_id, d.id, d.amount_cents, COALESCE(d.tag, ''), d.custom_shares::text
		  FROM items i
		  JOIN item_details d ON d.item_id = i.id
		 WHERE i.event_id = $1
		 ORDER BY i.id, d.ordinal`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []splitengine.Item{}
	byID := map[int64]*splitengine.Item{}
	for rows.Next() {
		var (
			itemID, payerID, detailID, amount int64
			tag, customText                   string
		)
		if err := rows.Scan(&itemID, &payerID, &detailID, &amount, &tag, &customText); err != nil {
			return nil, err
		}
		var custom map[string]int64
		if err := json.Unmarshal([]byte(customText), &custom); err != nil {
			return nil, err
		}
		it, ok := byID[itemID]
		if !ok {
			items = append(items, splitengine.Item{ID: itemID, PayerID: payerID})
			it = &items[len(items)-1]
			byID[itemID] = it
		}
		it.Details = append(it.Details, splitengine.Detail{
			ID:           detailID,
			AmountCents:  amount,
			Tag:          tag,
			CustomShares: numericStringMap(custom),
		})
	}
	return items, rows.Err()
}

func loadEngineRules(ctx context.Context, db *pgxpool.Pool, eventID int64) ([]splitengine.Rule, error) {
	rows, err := db.Query(ctx, `
		SELECT item_tag, groups::text, COALESCE(rest::text, '')
		  FROM event_rules
		 WHERE event_id = $1
		 ORDER BY ordinal, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type wireGroup struct {
		Conds  []string `json:"conds"`
		Mode   string   `json:"mode"`
		Weight float64  `json:"weight"`
	}
	out := []splitengine.Rule{}
	for rows.Next() {
		var tag, groupsText, restText string
		if err := rows.Scan(&tag, &groupsText, &restText); err != nil {
			return nil, err
		}
		var gs []wireGroup
		if err := json.Unmarshal([]byte(groupsText), &gs); err != nil {
			return nil, err
		}
		rule := splitengine.Rule{Tag: tag}
		for _, g := range gs {
			rule.Groups = append(rule.Groups, splitengine.Group{
				Conds: g.Conds, Mode: g.Mode, Weight: g.Weight,
			})
		}
		if restText != "" {
			var r wireGroup
			if err := json.Unmarshal([]byte(restText), &r); err != nil {
				return nil, err
			}
			rest := splitengine.Group{Mode: r.Mode, Weight: r.Weight}
			rule.Rest = &rest
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func numericStringMap(in map[string]int64) map[int64]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int64]int64, len(in))
	for k, v := range in {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		out[id] = v
	}
	return out
}

func sharesFromResult(r engineResult) sharesResponse {
	perMember := make([]memberShareDTO, 0, len(r.Shares.PerMember))
	ids := make([]int64, 0, len(r.Shares.PerMember))
	for id := range r.Shares.PerMember {
		ids = append(ids, id)
	}
	sortInt64s(ids)
	for _, id := range ids {
		ms := r.Shares.PerMember[id]
		perMember = append(perMember, memberShareDTO{
			MemberID: id, OwedCents: ms.OwedCents, PaidCents: ms.PaidCents, NetCents: ms.NetCents,
		})
	}
	perDetail := make([]detailShareDTO, 0, len(r.Shares.PerDetail))
	for _, ds := range r.Shares.PerDetail {
		shares := make(map[string]int64, len(ds.Shares))
		for id, cents := range ds.Shares {
			shares[strconv.FormatInt(id, 10)] = cents
		}
		perDetail = append(perDetail, detailShareDTO{
			ItemID: ds.ItemID, DetailID: ds.DetailID, AmountCents: ds.AmountCents, Shares: shares,
		})
	}
	return sharesResponse{
		GrandTotalCents: r.Shares.GrandTotalCents,
		PerMember:       perMember,
		PerDetail:       perDetail,
	}
}

func sortInt64s(a []int64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

type pairLineDTO struct {
	ItemID      int64 `json:"item_id"`
	DetailID    int64 `json:"detail_id"`
	FromID      int64 `json:"from_id"`
	ToID        int64 `json:"to_id"`
	AmountCents int64 `json:"amount_cents"`
}

type pairResponse struct {
	AID       int64         `json:"a_id"`
	BID       int64         `json:"b_id"`
	NetAOwesB int64         `json:"net_a_owes_b_cents"`
	Lines     []pairLineDTO `json:"lines"`
}

// GetPair godoc
// @Summary     Item-level breakdown of debt between two members
// @Description Any member may call. Returns every detail that contributes
// @Description to a direct debt between :a and :b, plus the signed net
// @Description (positive: a owes b, negative: b owes a).
// @Tags        shares
// @Produce     json
// @Param       id path int true "Event id"
// @Param       a  path int true "Member A id"
// @Param       b  path int true "Member B id"
// @Success     200 {object} pairResponse
// @Failure     400 {object} errorResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/pairs/{a}/{b} [get]
func (h *Handler) GetPair(c *gin.Context) {
	aID, ok := memberParam(c, "a")
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid member a id")
		return
	}
	bID, ok := memberParam(c, "b")
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid member b id")
		return
	}
	if aID == bID {
		respondErr(c, http.StatusBadRequest, "a and b must be different members")
		return
	}

	result, err := computeEventShares(c.Request.Context(), h.DB, eventIDFromPath(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "compute shares")
		return
	}
	rawLines := splitengine.PairBreakdown(result.Shares, result.Items, aID, bID)

	out := pairResponse{AID: aID, BID: bID, Lines: make([]pairLineDTO, 0, len(rawLines))}
	for _, l := range rawLines {
		out.Lines = append(out.Lines, pairLineDTO{
			ItemID: l.ItemID, DetailID: l.DetailID,
			FromID: l.FromID, ToID: l.ToID, AmountCents: l.AmountCents,
		})
		switch {
		case l.FromID == aID && l.ToID == bID:
			out.NetAOwesB += l.AmountCents
		case l.FromID == bID && l.ToID == aID:
			out.NetAOwesB -= l.AmountCents
		}
	}
	c.JSON(http.StatusOK, out)
}

func memberParam(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}
