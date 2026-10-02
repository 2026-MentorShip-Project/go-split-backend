package events

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"go-split-backend/internal/auth"
)

func (h *Handler) registerDetailRoutes(g *gin.RouterGroup) {
	g.PATCH("/:id", auth.RequireEventRole(h.DB, "host"), h.PatchEvent)
	g.DELETE("/:id", auth.RequireEventRole(h.DB, "host"), h.DeleteEvent)
	g.GET("/:id", auth.RequireEventRole(h.DB, "host", "co", "member"), h.GetEvent)
}

type eventDetailResponse struct {
	TransferNote string      `json:"transfer_note"`
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	Place        string      `json:"place"`
	Template     string      `json:"template"`
	StartsAt     string      `json:"starts_at"`
	EndsAt       string      `json:"ends_at"`
	Settled      bool        `json:"settled"`
	Archived     bool        `json:"archived"`
	CreatedAt    time.Time   `json:"created_at"`
	InviteCode   string      `json:"invite_code,omitempty"`
	Members      []memberDTO `json:"members"`
	Items        []itemDTO   `json:"items"`
	Total        int64       `json:"total"`
	MyRole       string      `json:"my_role"`
}

// GetEvent godoc
// @Summary     Get one event with members, items, and totals
// @Description Any member of the event may call. Returns the event
// @Description metadata, the current invite code, every member with role
// @Description and tags, every item card with its detail lines, and the
// @Description grand total in whole NT dollars so the event page's header can render
// @Description without a second round-trip.
// @Tags        events
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} eventDetailResponse
// @Failure     400 {object} errorResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Failure     404 {object} errorResponse
// @Router      /events/{id} [get]
func (h *Handler) GetEvent(c *gin.Context) {
	eventID := eventIDFromPath(c)
	ctx := c.Request.Context()

	base, err := loadEventBase(ctx, h.DB, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, http.StatusNotFound, "event not found")
			return
		}
		respondErr(c, http.StatusInternalServerError, "load event")
		return
	}

	sub := auth.CurrentSubject(c)
	members, err := loadMembers(ctx, h.DB, eventID, sub)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load members")
		return
	}
	items, err := loadItems(ctx, h.DB, eventID, 0)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load items")
		return
	}
	invite := ""
	if !base.Settled {
		invite, _ = loadInviteCode(ctx, h.DB, eventID)
	}

	var total int64
	for _, it := range items {
		total += it.Total
	}
	base.Members = members
	base.Items = items
	base.InviteCode = invite
	base.Total = total
	base.MyRole = auth.EventRole(c)

	c.JSON(http.StatusOK, base)
}

func loadEventBase(ctx context.Context, db database.Store, eventID int64) (eventDetailResponse, error) {
	var (
		out              eventDetailResponse
		startsAt, endsAt *time.Time
	)
	err := db.QueryRow(ctx, `
		SELECT id, name, place, template, starts_at, ends_at, settled, archived, created_at, transfer_note
		  FROM events
		 WHERE id = $1`, eventID).
		Scan(&out.ID, &out.Name, &out.Place,
			&out.Template, &startsAt, &endsAt, &out.Settled, &out.Archived, &out.CreatedAt, &out.TransferNote)
	out.StartsAt = formatDate(startsAt)
	out.EndsAt = formatDate(endsAt)
	return out, err
}

func loadMembers(ctx context.Context, db database.Store, eventID int64, sub auth.Subject) ([]memberDTO, error) {
	snap, err := loadSnapshot(ctx, db, eventID)
	if err != nil {
		return nil, err
	}
	if snap != nil {
		rows, err := db.Query(ctx, `
			SELECT id FROM event_members
			 WHERE event_id = $1
			   AND (($2 <> 0 AND account_id = $2) OR ($3 <> 0 AND guest_id = $3))`,
			eventID, sub.AccountID, sub.GuestID)
		if err != nil {
			return nil, err
		}
		mine, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return nil, err
		}
		out := snap.Event.Members
		for i := range out {
			out[i].You = slices.Contains(mine, out[i].ID)
		}
		return out, nil
	}

	rows, err := db.Query(ctx, `
		SELECT id, display, role::text, tags, note, split_order,
		       (guest_id IS NOT NULL) AS guest,
		       COALESCE(($1 <> 0 AND account_id = $1) OR ($2 <> 0 AND guest_id = $2), false) AS you
		  FROM event_members
		 WHERE event_id = $3
		 ORDER BY split_order, id`,
		sub.AccountID, sub.GuestID, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []memberDTO{}
	for rows.Next() {
		var m memberDTO
		if err := rows.Scan(&m.ID, &m.Display, &m.Role, &m.Tags, &m.Note, &m.SplitOrder, &m.Guest, &m.You); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadInviteCode(ctx context.Context, db database.Store, eventID int64) (string, error) {
	var code string
	err := db.QueryRow(ctx,
		`SELECT code FROM invitations WHERE event_id = $1 ORDER BY created_at DESC LIMIT 1`,
		eventID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return code, err
}
