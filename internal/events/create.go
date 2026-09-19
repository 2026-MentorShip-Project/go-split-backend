package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"go-split-backend/internal/auth"
)

// allowedTemplates includes placeholders that start with empty settings.
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
	Name     string `json:"name"                binding:"required,min=1,max=120"`
	Place    string `json:"place"               binding:"max=200"`
	Template string `json:"template"            binding:"required"`
}

type createEventResponse struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Place      string `json:"place"`
	Template   string `json:"template"`
	InviteCode string `json:"invite_code"`
}

// PostEvent godoc
// @Summary     Create a new event
// @Description Account-only. Persists the event, adds the account to event_members
// @Description with role 'host', and issues an invite code atomically.
// @Description Selected template tags and rules are applied in the same transaction.
// @Description Custom events start with empty settings. The
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

	if req.Name == "" {
		respondErr(c, 400, "name is required")
		return
	}
	if !allowedTemplates[req.Template] {
		respondErr(c, http.StatusBadRequest, "unknown template")
		return
	}

	sub := auth.CurrentSubject(c)
	if !sub.IsAccount() {
		respondErr(c, http.StatusForbidden, "only accounts can create events")
		return
	}

	ctx := c.Request.Context()
	out, err := createEventTx(ctx, h.DB, sub.AccountID, req)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "create event")
		return
	}
	c.JSON(http.StatusCreated, out)
}

// createEventTx runs the whole insert in one transaction so the event, the
// host membership row, template settings, and invite code are visible together or not at
// all.
func createEventTx(ctx context.Context, db database.Store, accountID int64, req createEventRequest) (createEventResponse, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return createEventResponse{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var content templateContent
	if req.Template != "自訂" {
		content, err = loadTemplateContent(ctx, tx, req.Template)
		if errors.Is(err, pgx.ErrNoRows) {
			content = templateContent{}
			err = nil
		}
		if err != nil {
			return createEventResponse{}, fmt.Errorf("load template: %w", err)
		}
	}

	var eventID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO events (account_id, name, place, template)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		accountID, req.Name, req.Place, req.Template,
	).Scan(&eventID)
	if err != nil {
		return createEventResponse{}, fmt.Errorf("insert event: %w", err)
	}

	if err := replaceEventSettingsTx(ctx, tx, eventID, content); err != nil {
		return createEventResponse{}, fmt.Errorf("apply template: %w", err)
	}

	display, err := lookupAccountName(ctx, tx, accountID)
	if err != nil {
		return createEventResponse{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_members (event_id, account_id, display, role)
		VALUES ($1, $2, $3, 'host')`,
		eventID, accountID, display); err != nil {
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
		Template:   req.Template,
		InviteCode: code,
	}, nil
}

func lookupAccountName(ctx context.Context, tx pgx.Tx, accountID int64) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM accounts WHERE id = $1`, accountID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("session account not found")
		}
		return "", fmt.Errorf("load account name: %w", err)
	}
	return name, nil
}

// txExec adapts pgx.Tx to the inviteExecer interface so issueInviteCode can
// run inside the create-event transaction.
type txExec struct{ tx pgx.Tx }

func (t txExec) Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}
