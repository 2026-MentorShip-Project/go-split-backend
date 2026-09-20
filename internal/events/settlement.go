package events

import (
	"context"
	"encoding/json"
	"errors"
	"go-split-backend/internal/auth"
	"go-split-backend/internal/database"
	"go-split-backend/internal/splitengine"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) registerSettlementRoutes(g *gin.RouterGroup) {
	host := auth.RequireEventRole(h.DB, "host")
	g.POST("/:id/settle", host, h.PostSettle)
	g.PATCH("/:id/settlement-note", host, h.PatchSettlementNote)
	g.POST("/:id/archive", host, h.PostArchive)
}

type settlementSnapshot struct {
	EngineVersion string                 `json:"engine_version"`
	Strategy      string                 `json:"strategy"`
	HubID         int64                  `json:"hub_id"`
	SplitOrder    []int64                `json:"split_order"`
	CreatedAt     time.Time              `json:"created_at"`
	Event         eventDetailResponse    `json:"event"`
	ItemTags      []string               `json:"item_tags"`
	CondTags      []string               `json:"cond_tags"`
	Rules         []ruleDTO              `json:"rules"`
	Engine        engineResult           `json:"engine"`
	Transfers     []splitengine.Transfer `json:"transfers"`
}

func loadSnapshot(ctx context.Context, db database.Store, id int64) (*settlementSnapshot, error) {
	var raw []byte
	if err := db.QueryRow(ctx, "SELECT settlement FROM events WHERE id=$1", id).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var s settlementSnapshot
	err := json.Unmarshal(raw, &s)
	return &s, err
}
func eventHub(ctx context.Context, db database.Store, id int64) (int64, error) {
	var hub int64
	var count int
	err := db.QueryRow(ctx, `SELECT count(*), COALESCE(min(m.id),0) FROM event_members m JOIN events e ON e.id=m.event_id WHERE m.event_id=$1 AND m.role='host' AND m.account_id=e.account_id`, id).Scan(&count, &hub)
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, errors.New("expected exactly one host")
	}
	return hub, nil
}

// PostSettle godoc
// @Summary Validate and permanently freeze an event
// @Description Host-only. Locks edits and invitation use; stores all inputs and results atomically.
// @Tags settlement
// @Produce json
// @Param id path int true "Event id"
// @Success 204
// @Failure 422 {object} validationResponse
// @Router /events/{id}/settle [post]
func (h *Handler) PostSettle(c *gin.Context) {
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	r, err := computeEventShares(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "compute settlement")
		return
	}
	if issues := splitIssues(r); len(issues) > 0 {
		c.JSON(422, validationResponse{Error: "invalid splits", Details: issues})
		return
	}
	hub, err := eventHub(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 409, err.Error())
		return
	}
	ts, err := splitengine.HubTransfers(r.Shares, hub)
	if err != nil {
		respondErr(c, 422, err.Error())
		return
	}
	base, err := loadEventBase(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "load event")
		return
	}
	base.Members, err = loadMembers(ctx, h.DB, id, auth.Subject{})
	if err != nil {
		respondErr(c, 500, "load members")
		return
	}
	base.Items, err = loadItems(ctx, h.DB, id, 0)
	if err != nil {
		respondErr(c, 500, "load items")
		return
	}
	base.Total = r.Shares.GrandTotal
	base.Settled = true
	base.InviteCode = ""
	itemTags, err := loadTagLabels(ctx, h.DB, id, "event_item_tags")
	if err != nil {
		respondErr(c, 500, "load item tags")
		return
	}
	condTags, err := loadTagLabels(ctx, h.DB, id, "event_cond_tags")
	if err != nil {
		respondErr(c, 500, "load condition tags")
		return
	}
	rules, err := loadRules(ctx, h.DB, id)
	if err != nil {
		respondErr(c, 500, "load rules")
		return
	}
	order := []int64{}
	for _, m := range r.Members {
		order = append(order, m.ID)
	}
	s := settlementSnapshot{splitengine.Version, "hub", hub, order, time.Now().UTC(), base, itemTags, condTags, rules, r, ts}
	raw, err := json.Marshal(s)
	if err != nil {
		respondErr(c, 500, "encode settlement")
		return
	}
	if _, err = h.DB.Exec(ctx, "UPDATE events SET settled=TRUE,settlement=$2 WHERE id=$1", id, raw); err != nil {
		respondErr(c, 500, "save settlement")
		return
	}
	c.Status(204)
}

// PostArchive godoc
// @Summary Archive a settled event without payment prerequisites
// @Tags settlement
// @Param id path int true "Event id"
// @Success 204
// @Router /events/{id}/archive [post]
func (h *Handler) PostArchive(c *gin.Context) {
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	var settled bool
	if err := h.DB.QueryRow(ctx, "SELECT settled FROM events WHERE id=$1", id).Scan(&settled); err != nil {
		respondErr(c, 500, "load event")
		return
	}
	if !settled {
		respondErr(c, 409, "event is not settled")
		return
	}
	if _, err := h.DB.Exec(ctx, "UPDATE events SET archived=TRUE WHERE id=$1", id); err != nil {
		respondErr(c, 500, "archive event")
		return
	}
	c.Status(204)
}
