package events

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

func (h *Handler) registerSettlementRoutes(g *gin.RouterGroup) {
	writeRole := auth.RequireEventRole(h.DB, "host", "co")
	hostOnly := auth.RequireEventRole(h.DB, "host")

	g.PUT("/:id/transfers/:from/:to/paid", writeRole, h.PutTransferPaid)
	g.DELETE("/:id/transfers/:from/:to/paid", writeRole, h.DeleteTransferPaid)
	g.POST("/:id/archive", hostOnly, h.PostArchive)
}

// PutTransferPaid godoc
// @Summary     Mark a transfer as paid
// @Description Host or co-organizer. The event must be settled; before
// @Description settle the transfer set is not stable and paid state has no
// @Description meaning. Idempotent.
// @Tags        settlement
// @Produce     json
// @Param       id   path int true "Event id"
// @Param       from path int true "From member id"
// @Param       to   path int true "To member id"
// @Success     204
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /events/{id}/transfers/{from}/{to}/paid [put]
func (h *Handler) PutTransferPaid(c *gin.Context) {
	eventID, fromID, toID, ok := parseTransferPath(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := requireSettled(ctx, h.DB, eventID); err != nil {
		respondSettlementErr(c, err)
		return
	}
	if err := ensureMembersInEvent(ctx, h.DB, eventID, fromID, toID); err != nil {
		respondSettlementErr(c, err)
		return
	}
	_, err := h.DB.Exec(ctx, `
		INSERT INTO transfer_payments (event_id, from_member_id, to_member_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (event_id, from_member_id, to_member_id) DO NOTHING`,
		eventID, fromID, toID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "mark transfer paid")
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteTransferPaid godoc
// @Summary     Undo a transfer's paid mark
// @Description Host or co-organizer. Requires settled. Idempotent.
// @Tags        settlement
// @Produce     json
// @Param       id   path int true "Event id"
// @Param       from path int true "From member id"
// @Param       to   path int true "To member id"
// @Success     204
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /events/{id}/transfers/{from}/{to}/paid [delete]
func (h *Handler) DeleteTransferPaid(c *gin.Context) {
	eventID, fromID, toID, ok := parseTransferPath(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := requireSettled(ctx, h.DB, eventID); err != nil {
		respondSettlementErr(c, err)
		return
	}
	_, err := h.DB.Exec(ctx,
		`DELETE FROM transfer_payments WHERE event_id = $1 AND from_member_id = $2 AND to_member_id = $3`,
		eventID, fromID, toID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "clear transfer paid")
		return
	}
	c.Status(http.StatusNoContent)
}

// PostArchive godoc
// @Summary     Archive a settled event
// @Description Host-only. Flips archived=true; the event and every child
// @Description row becomes read-only. Requires the event to already be
// @Description settled. Idempotent on an already-archived event.
// @Tags        settlement
// @Produce     json
// @Param       id path int true "Event id"
// @Success     204
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Failure     409 {object} errorResponse
// @Router      /events/{id}/archive [post]
func (h *Handler) PostArchive(c *gin.Context) {
	eventID := eventIDFromPath(c)
	ctx := c.Request.Context()
	if err := requireSettled(ctx, h.DB, eventID); err != nil {
		respondSettlementErr(c, err)
		return
	}
	if _, err := h.DB.Exec(ctx, `UPDATE events SET archived = TRUE WHERE id = $1`, eventID); err != nil {
		respondErr(c, http.StatusInternalServerError, "archive event")
		return
	}
	c.Status(http.StatusNoContent)
}

var (
	errEventNotFound    = errors.New("event not found")
	errEventNotSettled  = errors.New("event is not settled")
	errMemberNotInEvent = errors.New("member is not in this event")
	errTransferSelfLoop = errors.New("from and to must be different members")
)

func requireSettled(ctx context.Context, db *pgxpool.Pool, eventID int64) error {
	var settled bool
	err := db.QueryRow(ctx, `SELECT settled FROM events WHERE id = $1`, eventID).Scan(&settled)
	if errors.Is(err, pgx.ErrNoRows) {
		return errEventNotFound
	}
	if err != nil {
		return err
	}
	if !settled {
		return errEventNotSettled
	}
	return nil
}

func ensureMembersInEvent(ctx context.Context, db *pgxpool.Pool, eventID, fromID, toID int64) error {
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM event_members WHERE event_id = $1 AND id IN ($2, $3)`,
		eventID, fromID, toID).Scan(&count)
	if err != nil {
		return err
	}
	if count != 2 {
		return errMemberNotInEvent
	}
	return nil
}

func parseTransferPath(c *gin.Context) (eventID, fromID, toID int64, ok bool) {
	eventID = eventIDFromPath(c)
	from, err := strconv.ParseInt(c.Param("from"), 10, 64)
	if err != nil || from <= 0 {
		respondErr(c, http.StatusBadRequest, "invalid from member id")
		return 0, 0, 0, false
	}
	to, err := strconv.ParseInt(c.Param("to"), 10, 64)
	if err != nil || to <= 0 {
		respondErr(c, http.StatusBadRequest, "invalid to member id")
		return 0, 0, 0, false
	}
	if from == to {
		respondErr(c, http.StatusBadRequest, errTransferSelfLoop.Error())
		return 0, 0, 0, false
	}
	return eventID, from, to, true
}

func respondSettlementErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errEventNotFound):
		respondErr(c, http.StatusNotFound, "event not found")
	case errors.Is(err, errEventNotSettled):
		respondErr(c, http.StatusConflict, "event is not settled")
	case errors.Is(err, errMemberNotInEvent):
		respondErr(c, http.StatusNotFound, "member is not in this event")
	default:
		respondErr(c, http.StatusInternalServerError, "settlement operation failed")
	}
}

// loadPaidPairs returns the set of paid (from,to) member pairs for the
// event, keyed as "from>to" to match the frontend's transfer key format.
func loadPaidPairs(ctx context.Context, db *pgxpool.Pool, eventID int64) (map[string]bool, error) {
	rows, err := db.Query(ctx,
		`SELECT from_member_id, to_member_id FROM transfer_payments WHERE event_id = $1`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var from, to int64
		if err := rows.Scan(&from, &to); err != nil {
			return nil, err
		}
		out[transferKey(from, to)] = true
	}
	return out, rows.Err()
}

func transferKey(from, to int64) string {
	return strconv.FormatInt(from, 10) + ">" + strconv.FormatInt(to, 10)
}
