package events

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

// registerJoinRoutes is called from Handler.Register so the /events group
// carries the same session middleware.
func (h *Handler) registerJoinRoutes(g *gin.RouterGroup) {
	g.POST("/join", h.PostJoin)
}

type joinRequest struct {
	Code string `json:"code" binding:"required,min=1,max=32"`
}

type joinResponse struct {
	EventID int64  `json:"event_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

// PostJoin godoc
// @Summary     Join an existing event by invite code
// @Description Attach the current session's user to the event named by the
// @Description code as a participant. Idempotent: an existing membership
// @Description keeps its role (a co-organizer promoted by the host stays
// @Description a co-organizer). Rejects codes for settled events with 410.
// @Tags        events
// @Accept      json
// @Produce     json
// @Param       body body     joinRequest true "Invite code"
// @Success     200  {object} joinResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     404  {object} errorResponse
// @Failure     410  {object} errorResponse
// @Router      /events/join [post]
func (h *Handler) PostJoin(c *gin.Context) {
	var req joinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)

	sub := auth.CurrentSubject(c)
	if !sub.IsAccount() && !sub.IsGuest() {
		respondErr(c, http.StatusUnauthorized, "not signed in")
		return
	}

	ctx := c.Request.Context()
	eventID, name, settled, ownerAccountID, err := lookupInvite(ctx, h.DB, req.Code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, http.StatusNotFound, "invite code not found")
			return
		}
		respondErr(c, http.StatusInternalServerError, "lookup invite")
		return
	}
	if settled {
		respondErr(c, http.StatusGone, "invite code has expired")
		return
	}

	// An account that owns the event is already the host; nothing to insert.
	if sub.IsAccount() && sub.AccountID == ownerAccountID {
		c.JSON(http.StatusOK, joinResponse{EventID: eventID, Name: name, Role: "host"})
		return
	}

	role, err := attachToEvent(ctx, h.DB, eventID, sub, displayNameFor(sub))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "attach to event")
		return
	}
	c.JSON(http.StatusOK, joinResponse{EventID: eventID, Name: name, Role: role})
}

func lookupInvite(ctx context.Context, db *pgxpool.Pool, code string) (int64, string, bool, int64, error) {
	var (
		eventID        int64
		name           string
		settled        bool
		ownerAccountID int64
	)
	err := db.QueryRow(ctx, `
		SELECT e.id, e.name, e.settled, e.account_id
		  FROM invitations i
		  JOIN events      e ON e.id = i.event_id
		 WHERE i.code = $1`, code).Scan(&eventID, &name, &settled, &ownerAccountID)
	return eventID, name, settled, ownerAccountID, err
}

func attachToEvent(ctx context.Context, db *pgxpool.Pool, eventID int64, sub auth.Subject, display string) (string, error) {
	var accountID, guestID any
	var conflict string
	switch {
	case sub.IsAccount():
		accountID = sub.AccountID
		conflict = "(event_id, account_id)"
	case sub.IsGuest():
		guestID = sub.GuestID
		conflict = "(event_id, guest_id)"
	default:
		return "", errors.New("subject has neither account nor guest id")
	}

	var role string
	err := db.QueryRow(ctx, `
		INSERT INTO event_members (event_id, account_id, guest_id, display, role)
		VALUES ($1, $2, $3, $4, 'member')
		ON CONFLICT `+conflict+` DO UPDATE SET display = event_members.display
		RETURNING role::text`, eventID, accountID, guestID, display).Scan(&role)
	return role, err
}

// displayNameFor is a placeholder that will be replaced when account/guest name
// lookups land. For now it labels the row by subject type so QA can see
// which side attached.
func displayNameFor(sub auth.Subject) string {
	if sub.IsAccount() {
		return "account"
	}
	return "guest"
}
