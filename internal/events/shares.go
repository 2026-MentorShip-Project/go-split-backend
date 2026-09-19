package events

import (
	"context"
	"encoding/json"
	"go-split-backend/internal/auth"
	"go-split-backend/internal/database"
	"go-split-backend/internal/splitengine"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *Handler) registerSharesRoutes(g *gin.RouterGroup) {
	anyRole := auth.RequireEventRole(h.DB, "host", "co", "member")
	g.GET("/:id/shares", anyRole, h.GetShares)
	g.GET("/:id/transfers", anyRole, h.GetTransfers)
	g.GET("/:id/me/details", anyRole, h.GetPersonalDetails)
	g.GET("/:id/members/:member_id/breakdown", auth.RequireEventRole(h.DB, "host"), h.GetMemberBreakdown)
}

type memberShareDTO struct {
	MemberID int64 `json:"member_id"`
	Owed     int64 `json:"owed"`
	Paid     int64 `json:"advanced"`
	Net      int64 `json:"net"`
}
type detailShareDTO struct {
	ItemID   int64 `json:"item_id"`
	DetailID int64 `json:"detail_id"`
	Amount   int64 `json:"amount"`
	splitengine.SplitResult
}
type sharesResponse struct {
	GrandTotal int64            `json:"grand_total"`
	PerMember  []memberShareDTO `json:"per_member"`
	PerDetail  []detailShareDTO `json:"per_detail"`
}
type transfersResponse struct {
	Strategy  string                 `json:"strategy"`
	HubID     int64                  `json:"hub_id"`
	Transfers []splitengine.Transfer `json:"transfers"`
}
type engineResult struct {
	Members []splitengine.Member
	Items   []splitengine.Item
	Rules   []splitengine.Rule
	Shares  splitengine.Shares
}

func computeEventShares(ctx context.Context, db database.Store, eventID int64) (engineResult, error) {
	snap, err := loadSnapshot(ctx, db, eventID)
	if err != nil {
		return engineResult{}, err
	}
	if snap != nil {
		return snap.Engine, nil
	}
	ms, err := loadEngineMembers(ctx, db, eventID)
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
	return engineResult{ms, items, rules, splitengine.Compute(ms, items, rules)}, nil
}

// GetShares godoc
// @Summary Get split results (personal unless host or archived)
// @Tags shares
// @Produce json
// @Param id path int true "Event id"
// @Success 200 {object} sharesResponse
// @Router /events/{id}/shares [get]
func (h *Handler) GetShares(c *gin.Context) {
	r, err := computeEventShares(c.Request.Context(), h.DB, eventIDFromPath(c))
	if err != nil {
		respondErr(c, 500, "compute shares")
		return
	}
	all, err := allResultsVisible(c, h.DB)
	if err != nil {
		respondErr(c, 500, "load visibility")
		return
	}
	filter := int64(0)
	if !all {
		filter = auth.EventMemberID(c)
	}
	c.JSON(200, sharesFromResult(r, filter))
}
func allResultsVisible(c *gin.Context, db database.Store) (bool, error) {
	if auth.EventRole(c) == "host" {
		return true, nil
	}
	var archived bool
	err := db.QueryRow(c.Request.Context(), "SELECT archived FROM events WHERE id=$1", eventIDFromPath(c)).Scan(&archived)
	return archived, err
}

// GetTransfers godoc
// @Summary Get host-routed transfers (host only until archived)
// @Tags shares
// @Produce json
// @Param id path int true "Event id"
// @Success 200 {object} transfersResponse
// @Router /events/{id}/transfers [get]
func (h *Handler) GetTransfers(c *gin.Context) {
	all, err := allResultsVisible(c, h.DB)
	if err != nil {
		respondErr(c, 500, "load visibility")
		return
	}
	if !all {
		respondErr(c, 403, "full transfers are host-only until archived")
		return
	}
	if c.Query("hub") != "" || c.Query("mode") != "" {
		respondErr(c, 400, "strategy and host are server-managed")
		return
	}
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	snap, err := loadSnapshot(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "load settlement")
		return
	}
	if snap != nil {
		c.JSON(200, transfersResponse{"hub", snap.HubID, snap.Transfers})
		return
	}
	r, err := computeEventShares(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "compute shares")
		return
	}
	if issues := splitIssues(r); len(issues) > 0 {
		c.JSON(422, validationResponse{Error: "invalid splits", Details: issues})
		return
	}
	hub, err := eventHub(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 409, "event must have one account-backed host")
		return
	}
	ts, err := splitengine.HubTransfers(r.Shares, hub)
	if err != nil {
		respondErr(c, 422, err.Error())
		return
	}
	c.JSON(200, transfersResponse{"hub", hub, ts})
}
func sharesFromResult(r engineResult, filter int64) sharesResponse {
	out := sharesResponse{GrandTotal: r.Shares.GrandTotal, PerMember: []memberShareDTO{}, PerDetail: []detailShareDTO{}}
	for _, m := range r.Members {
		if filter != 0 && filter != m.ID {
			continue
		}
		ms := r.Shares.PerMember[m.ID]
		out.PerMember = append(out.PerMember, memberShareDTO{m.ID, ms.Owed, ms.Paid, ms.Net})
	}
	for _, ds := range r.Shares.PerDetail {
		sr := ds.Result
		if filter != 0 {
			sr.Shares = filterShares(sr.Shares, filter)
			sr.Excluded = filterShares(sr.Excluded, filter)
		}
		out.PerDetail = append(out.PerDetail, detailShareDTO{ds.ItemID, ds.DetailID, ds.Amount, sr})
	}
	return out
}
func filterShares(in []splitengine.Share, id int64) []splitengine.Share {
	out := []splitengine.Share{}
	for _, s := range in {
		if s.MemberID == id {
			out = append(out, s)
		}
	}
	return out
}

type personalLine struct {
	ItemID   int64  `json:"item_id"`
	DetailID int64  `json:"detail_id"`
	Name     string `json:"name"`
	PayerID  int64  `json:"payer_id"`
	Owed     int64  `json:"owed"`
	Advanced int64  `json:"advanced"`
	Net      int64  `json:"net"`
}
type personalResponse struct {
	MemberID  int64                  `json:"member_id"`
	Net       int64                  `json:"net"`
	Lines     []personalLine         `json:"lines"`
	Transfers []splitengine.Transfer `json:"transfers"`
}

// GetPersonalDetails godoc
// @Summary Get every detail contributing to the caller's hub balance, including zeros
// @Tags shares
// @Produce json
// @Param id path int true "Event id"
// @Success 200 {object} personalResponse
// @Router /events/{id}/me/details [get]
func (h *Handler) GetPersonalDetails(c *gin.Context) {
	memberID := auth.EventMemberID(c)
	if c.Param("member_id") != "" {
		v, ok := memberIDFromPath(c)
		if !ok {
			respondErr(c, 400, "invalid member id")
			return
		}
		memberID = v
	}
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	r, err := computeEventShares(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "compute shares")
		return
	}
	ms, ok := r.Shares.PerMember[memberID]
	if !ok {
		respondErr(c, 404, "member not found")
		return
	}
	items, err := loadItems(ctx, h.DB, id, 0)
	if err != nil {
		respondErr(c, 500, "load items")
		return
	}
	byDetail := map[int64]splitengine.DetailShares{}
	for _, d := range r.Shares.PerDetail {
		byDetail[d.DetailID] = d
	}
	out := personalResponse{MemberID: memberID, Net: ms.Net, Lines: []personalLine{}, Transfers: []splitengine.Transfer{}}
	for _, it := range items {
		for _, d := range it.Details {
			owed := byDetail[d.ID].Shares[memberID]
			advanced := int64(0)
			if it.PayerMemberID == memberID {
				advanced = d.Amount
			}
			out.Lines = append(out.Lines, personalLine{it.ID, d.ID, d.Name, it.PayerMemberID, owed, advanced, advanced - owed})
		}
	}
	snap, err := loadSnapshot(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "load settlement")
		return
	}
	if snap != nil {
		for _, tr := range snap.Transfers {
			if tr.FromID == memberID || tr.ToID == memberID {
				out.Transfers = append(out.Transfers, tr)
			}
		}
	}
	c.JSON(http.StatusOK, out)
}
func loadEngineMembers(ctx context.Context, db database.Store, eventID int64) ([]splitengine.Member, error) {
	rows, err := db.Query(ctx,
		`SELECT id, tags FROM event_members WHERE event_id = $1 ORDER BY split_order, id`, eventID)
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

func loadEngineItems(ctx context.Context, db database.Store, eventID int64) ([]splitengine.Item, error) {
	rows, err := db.Query(ctx, `
		SELECT i.id, i.payer_member_id, d.id, d.amount, COALESCE(d.tag, ''), d.custom_shares::text, d.manual_member_ids
		  FROM items i
		  JOIN item_details d ON d.item_id = i.id
		 WHERE i.event_id = $1
		 ORDER BY i.id, d.ordinal`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []splitengine.Item{}
	byID := map[int64]int{}
	for rows.Next() {
		var (
			itemID, payerID, detailID, amount int64
			tag, customText                   string
			manual                            []int64
		)
		if err := rows.Scan(&itemID, &payerID, &detailID, &amount, &tag, &customText, &manual); err != nil {
			return nil, err
		}
		var custom map[string]int64
		if err := json.Unmarshal([]byte(customText), &custom); err != nil {
			return nil, err
		}
		idx, ok := byID[itemID]
		if !ok {
			items = append(items, splitengine.Item{ID: itemID, PayerID: payerID})
			idx = len(items) - 1
			byID[itemID] = idx
		}
		it := &items[idx]
		it.Details = append(it.Details, splitengine.Detail{
			ID:              detailID,
			Amount:          amount,
			Tag:             tag,
			CustomShares:    numericStringMap(custom),
			ManualMemberIDs: manual,
		})
	}
	return items, rows.Err()
}

func loadEngineRules(ctx context.Context, db database.Store, eventID int64) ([]splitengine.Rule, error) {
	rows, err := db.Query(ctx, `
		SELECT item_tag, groups::text, COALESCE(rest::text, '')
		  FROM event_rules
		 WHERE event_id = $1
		 ORDER BY ordinal, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []splitengine.Rule{}
	for rows.Next() {
		var tag, groupsText, restText string
		if err := rows.Scan(&tag, &groupsText, &restText); err != nil {
			return nil, err
		}
		var gs []splitengine.Group
		if err := json.Unmarshal([]byte(groupsText), &gs); err != nil {
			return nil, err
		}
		rule := splitengine.Rule{Tag: tag}
		for _, g := range gs {
			rule.Groups = append(rule.Groups, splitengine.Group{
				Conds: g.Conds, Mode: g.Mode, Weight: g.Weight,
			})
		}
		if restText != "" && restText != "null" {
			var r splitengine.Group
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

// GetMemberBreakdown godoc
// @Summary Host-only breakdown of one member's complete detail ledger
// @Tags shares
// @Produce json
// @Param id path int true "Event id"
// @Param member_id path int true "Member id"
// @Success 200 {object} personalResponse
// @Router /events/{id}/members/{member_id}/breakdown [get]
func (h *Handler) GetMemberBreakdown(c *gin.Context) { h.GetPersonalDetails(c) }
