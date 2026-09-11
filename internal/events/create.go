package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

// allowedTemplates is the fixed list from data.js. Only the two "real" ones
// have designed rule content today, but every label is accepted so the
// frontend does not have to filter.
var allowedTemplates = map[string]bool{
	"自訂":      true,
	"烤肉/露營模板": true,
	"聚餐模板":    true,
	"唱歌模板":    true,
	"出國旅遊模板":  true,
	"社團活動模板":  true,
}

// registerCreateRoutes is called from Handler.Register so the /events group
// carries the same session middleware.
func (h *Handler) registerCreateRoutes(g *gin.RouterGroup) {
	g.POST("", h.PostEvent)
}

type createEventRequest struct {
	Name     string     `json:"name"                binding:"required,min=1,max=120"`
	Place    string     `json:"place"               binding:"max=200"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	Template string     `json:"template"            binding:"required"`
}

type createEventResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Place      string     `json:"place"`
	StartsAt   *time.Time `json:"starts_at,omitempty"`
	EndsAt     *time.Time `json:"ends_at,omitempty"`
	Template   string     `json:"template"`
	InviteCode string     `json:"invite_code"`
}

// PostEvent godoc
// @Summary     Create a new event
// @Description Host-only. Persists the event, adds the host to event_members
// @Description with role 'host', and issues an invite code atomically. The
// @Description response carries the code so the client can jump straight to
// @Description the invite screen without a second round-trip.
// @Tags        events
// @Accept      json
// @Produce     json
// @Param       body body     createEventRequest true "New event"
// @Success     201  {object} createEventResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Router      /events [post]
func (h *Handler) PostEvent(c *gin.Context) {
	var req createEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Place = strings.TrimSpace(req.Place)
	req.Template = strings.TrimSpace(req.Template)

	if !allowedTemplates[req.Template] {
		respondErr(c, http.StatusBadRequest, "unknown template")
		return
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		respondErr(c, http.StatusBadRequest, "ends_at must not be before starts_at")
		return
	}

	sub := auth.CurrentSubject(c)
	if !sub.IsHost() {
		respondErr(c, http.StatusForbidden, "only hosts can create events")
		return
	}

	ctx := c.Request.Context()
	out, err := createEventTx(ctx, h.DB, sub.HostID, req)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "create event")
		return
	}
	c.JSON(http.StatusCreated, out)
}

// createEventTx runs the whole insert in one transaction so the event, the
// host membership row, and the invite code are visible together or not at
// all.
func createEventTx(ctx context.Context, db *pgxpool.Pool, hostID int64, req createEventRequest) (createEventResponse, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return createEventResponse{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var eventID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO events (host_id, name, place, starts_at, ends_at, template)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		hostID, req.Name, req.Place, req.StartsAt, req.EndsAt, req.Template,
	).Scan(&eventID)
	if err != nil {
		return createEventResponse{}, fmt.Errorf("insert event: %w", err)
	}

	display, err := lookupHostName(ctx, tx, hostID)
	if err != nil {
		return createEventResponse{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_members (event_id, host_id, display, role)
		VALUES ($1, $2, $3, 'host')`,
		eventID, hostID, display); err != nil {
		return createEventResponse{}, fmt.Errorf("attach host: %w", err)
	}

	code, err := issueInviteCode(ctx, txExec{tx}, eventID)
	if err != nil {
		return createEventResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return createEventResponse{}, fmt.Errorf("commit tx: %w", err)
	}

	return createEventResponse{
		ID:         eventID,
		Name:       req.Name,
		Place:      req.Place,
		StartsAt:   req.StartsAt,
		EndsAt:     req.EndsAt,
		Template:   req.Template,
		InviteCode: code,
	}, nil
}

func lookupHostName(ctx context.Context, tx pgx.Tx, hostID int64) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM hosts WHERE id = $1`, hostID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("session host not found")
		}
		return "", fmt.Errorf("load host name: %w", err)
	}
	return name, nil
}

// txExec adapts pgx.Tx to the inviteExecer interface so issueInviteCode can
// run inside the create-event transaction.
type txExec struct{ tx pgx.Tx }

func (t txExec) Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}
