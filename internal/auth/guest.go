package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// registerGuestRoutes wires the guest and logout endpoints. Called from
// Handler.Register so the whole /auth group is set up in one place.
func (h *Handler) registerGuestRoutes(g *gin.RouterGroup) {
	g.GET("/invite/:code", h.GetInvitation)
	g.POST("/join", h.PostJoin)
	g.POST("/recover", h.PostRecover)
	g.POST("/logout", h.PostLogout)
}

type joinRequest struct {
	Code     string   `json:"code"  binding:"required,min=1,max=32"`
	Email    string   `json:"email" binding:"required,email"`
	Phone    string   `json:"phone" binding:"required,min=1,max=32"`
	Name     string   `json:"name" binding:"required,max=64"`
	CondTags []string `json:"cond_tags"`
	Note     string   `json:"note" binding:"max=1000"`
}

type joinResponse struct {
	GuestID int64  `json:"guest_id"`
	EventID int64  `json:"event_id"`
	Role    string `json:"role"`
}

// PostJoin godoc
// @Summary     Guest first-join by invite code
// @Description Look up the invite code, create a guest identity if the
// @Description email + phone combination is new, attach the guest to the
// @Description event as a participant, and set a session cookie. The
// @Description invite code is only accepted while the event is not settled.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     joinRequest true "Guest first-join"
// @Success     200  {object} joinResponse
// @Failure     400  {object} errorResponse
// @Failure     404  {object} errorResponse
// @Failure     410  {object} errorResponse
// @Router      /auth/join [post]
func (h *Handler) PostJoin(c *gin.Context) {
	var req joinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || !validPhone(req.Phone) {
		respondErr(c, 400, "name and numeric phone are required")
		return
	}

	ctx := c.Request.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		respondErr(c, 500, "begin join")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	eventID, settled, err := lookupInvite(ctx, tx, req.Code)
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

	if err := validateJoinConditions(ctx, tx, eventID, req.CondTags); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	guestID, err := upsertGuest(ctx, tx, req.Email, req.Phone, req.Name)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "create guest")
		return
	}

	role, err := attachGuestToEvent(ctx, tx, eventID, guestID, req.Name, req.CondTags, req.Note)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "attach guest to event")
		return
	}

	if _, err := IssueSession(ctx, tx, c, Subject{GuestID: guestID}); err != nil {
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		respondErr(c, 500, "commit join")
		return
	}
	c.JSON(http.StatusOK, joinResponse{GuestID: guestID, EventID: eventID, Role: role})
}

type recoverRequest struct {
	Code  string `json:"code"  binding:"required,min=1,max=32"`
	Email string `json:"email" binding:"required,email"`
	Phone string `json:"phone" binding:"required,min=1,max=32"`
}

type recoverResponse struct {
	GuestID int64 `json:"guest_id"`
	EventID int64 `json:"event_id"`
}

// PostRecover godoc
// @Summary     Recover a guest session by invite code
// @Description All three of invite code, email, and phone must match an
// @Description existing guest membership. On success a new session cookie
// @Description is issued for the original guest id. Only valid while the
// @Description event is not settled.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     recoverRequest true "Guest session recovery"
// @Success     200  {object} recoverResponse
// @Failure     400  {object} errorResponse
// @Failure     404  {object} errorResponse
// @Failure     410  {object} errorResponse
// @Router      /auth/recover [post]
func (h *Handler) PostRecover(c *gin.Context) {
	var req recoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)

	ctx := c.Request.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		respondErr(c, 500, "begin join")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	eventID, settled, err := lookupInvite(ctx, tx, req.Code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, http.StatusNotFound, "recovery failed")
			return
		}
		respondErr(c, http.StatusInternalServerError, "lookup invite")
		return
	}
	if settled {
		respondErr(c, http.StatusGone, "invite code has expired")
		return
	}

	var guestID int64
	err = tx.QueryRow(ctx, `
		SELECT g.id
		  FROM guests g
		  JOIN event_members em ON em.guest_id = g.id
		 WHERE em.event_id = $1 AND g.email = $2 AND g.phone = $3`,
		eventID, req.Email, req.Phone).Scan(&guestID)
	if errors.Is(err, pgx.ErrNoRows) {
		// PRD 6.2: any of the three not matching fails as one "recovery failed".
		respondErr(c, http.StatusNotFound, "recovery failed")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load guest")
		return
	}

	if _, err := IssueSession(ctx, tx, c, Subject{GuestID: guestID}); err != nil {
		respondErr(c, http.StatusInternalServerError, "issue session")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		respondErr(c, 500, "commit recovery")
		return
	}
	c.JSON(http.StatusOK, recoverResponse{GuestID: guestID, EventID: eventID})
}

// PostLogout godoc
// @Summary     End the current session
// @Description Deletes the session row named by the cookie and clears the
// @Description cookie. Safe to call without a cookie.
// @Tags        auth
// @Produce     json
// @Success     204 "no content"
// @Router      /auth/logout [post]
func (h *Handler) PostLogout(c *gin.Context) {
	if err := RevokeSession(c.Request.Context(), h.DB, c); err != nil {
		respondErr(c, http.StatusInternalServerError, "revoke session")
		return
	}
	c.Status(http.StatusNoContent)
}

func lookupInvite(ctx context.Context, db database.Store, code string) (int64, bool, error) {
	var (
		eventID int64
		settled bool
	)
	err := db.QueryRow(ctx, `
		SELECT e.id, e.settled
		  FROM invitations i
		  JOIN events      e ON e.id = i.event_id
		 WHERE i.code = $1 FOR UPDATE OF e`, code).Scan(&eventID, &settled)
	return eventID, settled, err
}

func upsertGuest(ctx context.Context, db database.Store, email, phone, name string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO guests (email, phone, name) VALUES ($1, $2, $3)
		ON CONFLICT (email, phone) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, email, phone, name).Scan(&id)
	return id, err
}

// attachGuestToEvent inserts a membership row if one does not exist for this
// guest on this event, defaulting to 'member' role. If the guest is already
// attached (perhaps as a co-organizer promoted by the host), the existing
// role is returned unchanged.
func attachGuestToEvent(ctx context.Context, db database.Store, eventID, guestID int64, display string, tags []string, note string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	var role string
	err := db.QueryRow(ctx, `
		INSERT INTO event_members (event_id, guest_id, display, role, tags, note)
		VALUES ($1, $2, $3, 'member', $4, $5)
		ON CONFLICT (event_id, guest_id) DO UPDATE SET display = event_members.display
		RETURNING role::text`, eventID, guestID, display, tags, note).Scan(&role)
	return role, err
}

func validPhone(phone string) bool {
	if phone == "" {
		return false
	}
	for _, r := range phone {
		if r < '0' || r > '9' || !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
func validateJoinConditions(ctx context.Context, db database.Store, id int64, tags []string) error {
	seen := map[string]bool{}
	for _, tag := range tags {
		if seen[tag] {
			return errors.New("duplicate condition")
		}
		seen[tag] = true
		var exists bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM event_cond_tags WHERE event_id=$1 AND label=$2)", id, tag).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errors.New("unknown condition")
		}
	}
	return nil
}
