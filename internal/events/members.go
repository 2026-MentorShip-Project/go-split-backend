package events

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"go-split-backend/internal/auth"
)

func (h *Handler) registerMemberRoutes(g *gin.RouterGroup) {
	anyRole := auth.RequireEventRole(h.DB, "host", "co", "member")
	hostOnly := auth.RequireEventRole(h.DB, "host")

	g.GET("/:id/members", anyRole, h.GetMembers)
	g.GET("/:id/members/:member_id/role", anyRole, h.GetMemberRole)
	g.POST("/:id/members", hostOnly, h.PostMember)
	g.PATCH("/:id/members/:member_id", hostOnly, h.PatchMember)
	g.DELETE("/:id/members/:member_id", hostOnly, h.DeleteMember)
}

type memberDTO struct {
	ID      int64    `json:"id"`
	Display string   `json:"display"`
	Role    string   `json:"role"`
	Tags    []string `json:"tags"`
	Guest   bool     `json:"guest"`
	You     bool     `json:"you,omitempty"`
}

type membersResponse struct {
	Members []memberDTO `json:"members"`
}

type roleResponse struct {
	Role string `json:"role"`
}

// GetMembers godoc
// @Summary     List members of an event
// @Description Any member of the event may call. Each row carries role,
// @Description condition tags, guest flag, and a "you" marker on the
// @Description caller's own row.
// @Tags        members
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} membersResponse
// @Failure     400 {object} errorResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/members [get]
func (h *Handler) GetMembers(c *gin.Context) {
	eventID := eventIDFromPath(c)
	sub := auth.CurrentSubject(c)

	rows, err := h.DB.Query(c.Request.Context(), `
		SELECT id, display, role::text, tags,
		       (guest_id IS NOT NULL) AS guest,
		       COALESCE(($1 <> 0 AND host_id = $1) OR ($2 <> 0 AND guest_id = $2), false) AS you
		  FROM event_members
		 WHERE event_id = $3
		 ORDER BY id`,
		sub.HostID, sub.GuestID, eventID)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list members")
		return
	}
	defer rows.Close()

	out := []memberDTO{}
	for rows.Next() {
		var m memberDTO
		if err := rows.Scan(&m.ID, &m.Display, &m.Role, &m.Tags, &m.Guest, &m.You); err != nil {
			respondErr(c, http.StatusInternalServerError, "scan member")
			return
		}
		out = append(out, m)
	}
	c.JSON(http.StatusOK, membersResponse{Members: out})
}

type createMemberRequest struct {
	Display string   `json:"display" binding:"required,min=1,max=64"`
	Role    string   `json:"role"    binding:"required,oneof=host co member"`
	Tags    []string `json:"tags"`
}

// PostMember godoc
// @Summary     Add a placeholder member to an event
// @Description Host-only. Creates a seat-holder row with no host or guest
// @Description identity yet; use invite codes to bind a real session.
// @Tags        members
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "Event id"
// @Param       body body createMemberRequest true "New member"
// @Success     201  {object} memberDTO
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Router      /events/{id}/members [post]
func (h *Handler) PostMember(c *gin.Context) {
	var req createMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	var m memberDTO
	err := h.DB.QueryRow(c.Request.Context(), `
		INSERT INTO event_members (event_id, display, role, tags)
		VALUES ($1, $2, $3::event_role, $4)
		RETURNING id, display, role::text, tags, (guest_id IS NOT NULL) AS guest`,
		eventID, req.Display, req.Role, tags,
	).Scan(&m.ID, &m.Display, &m.Role, &m.Tags, &m.Guest)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "create member")
		return
	}
	c.JSON(http.StatusCreated, m)
}

type updateMemberRequest struct {
	Display *string   `json:"display,omitempty"`
	Role    *string   `json:"role,omitempty" binding:"omitempty,oneof=host co member"`
	Tags    *[]string `json:"tags,omitempty"`
}

// PatchMember godoc
// @Summary     Update a member
// @Description Host-only. Partial update of display name, role, or tags.
// @Tags        members
// @Accept      json
// @Produce     json
// @Param       id        path int                  true "Event id"
// @Param       member_id path int                  true "Member id"
// @Param       body      body updateMemberRequest true "Fields to change"
// @Success     200       {object} memberDTO
// @Failure     400       {object} errorResponse
// @Failure     401       {object} errorResponse
// @Failure     403       {object} errorResponse
// @Failure     404       {object} errorResponse
// @Router      /events/{id}/members/{member_id} [patch]
func (h *Handler) PatchMember(c *gin.Context) {
	var req updateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	memberID, ok := memberIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid member id")
		return
	}

	var (
		displayArg any
		roleArg    any
		tagsArg    any
	)
	if req.Display != nil {
		displayArg = *req.Display
	}
	if req.Role != nil {
		roleArg = *req.Role
	}
	if req.Tags != nil {
		tagsArg = *req.Tags
	}

	var m memberDTO
	err := h.DB.QueryRow(c.Request.Context(), `
		UPDATE event_members
		   SET display = COALESCE($1, display),
		       role    = COALESCE($2::event_role, role),
		       tags    = COALESCE($3, tags)
		 WHERE id = $4 AND event_id = $5
	 RETURNING id, display, role::text, tags, (guest_id IS NOT NULL) AS guest`,
		displayArg, roleArg, tagsArg, memberID, eventID,
	).Scan(&m.ID, &m.Display, &m.Role, &m.Tags, &m.Guest)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "update member")
		return
	}
	c.JSON(http.StatusOK, m)
}

// DeleteMember godoc
// @Summary     Remove a member
// @Description Host-only. Refuses to remove the sole host of the event.
// @Tags        members
// @Produce     json
// @Param       id        path int true "Event id"
// @Param       member_id path int true "Member id"
// @Success     204       "no content"
// @Failure     400       {object} errorResponse
// @Failure     401       {object} errorResponse
// @Failure     403       {object} errorResponse
// @Failure     404       {object} errorResponse
// @Failure     409       {object} errorResponse
// @Router      /events/{id}/members/{member_id} [delete]
func (h *Handler) DeleteMember(c *gin.Context) {
	eventID := eventIDFromPath(c)
	memberID, ok := memberIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid member id")
		return
	}

	ctx := c.Request.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "begin tx")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role string
	err = tx.QueryRow(ctx,
		`SELECT role::text FROM event_members WHERE id = $1 AND event_id = $2`,
		memberID, eventID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load member")
		return
	}

	if role == "host" {
		var hostCount int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM event_members WHERE event_id = $1 AND role = 'host'`,
			eventID).Scan(&hostCount); err != nil {
			respondErr(c, http.StatusInternalServerError, "count hosts")
			return
		}
		if hostCount <= 1 {
			respondErr(c, http.StatusConflict, "cannot remove the sole host")
			return
		}
	}

	var itemCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM items
		 WHERE event_id = $1
		   AND (payer_member_id = $2 OR author_member_id = $2)`,
		eventID, memberID).Scan(&itemCount); err != nil {
		respondErr(c, http.StatusInternalServerError, "count items")
		return
	}
	if itemCount > 0 {
		respondErr(c, http.StatusConflict, "cannot remove a member who owns items")
		return
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM event_members WHERE id = $1 AND event_id = $2`,
		memberID, eventID); err != nil {
		respondErr(c, http.StatusInternalServerError, "delete member")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		respondErr(c, http.StatusInternalServerError, "commit")
		return
	}
	c.Status(http.StatusNoContent)
}

// GetMemberRole godoc
// @Summary     Get a member's role in an event
// @Description Any member of the event may call. Returns the role stored on
// @Description the event_members row (host, co, or member).
// @Tags        members
// @Produce     json
// @Param       id        path int true "Event id"
// @Param       member_id path int true "Member id"
// @Success     200       {object} roleResponse
// @Failure     400       {object} errorResponse
// @Failure     401       {object} errorResponse
// @Failure     403       {object} errorResponse
// @Failure     404       {object} errorResponse
// @Router      /events/{id}/members/{member_id}/role [get]
func (h *Handler) GetMemberRole(c *gin.Context) {
	eventID := eventIDFromPath(c)
	memberID, ok := memberIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid member id")
		return
	}

	var role string
	err := h.DB.QueryRow(c.Request.Context(),
		`SELECT role::text FROM event_members WHERE id = $1 AND event_id = $2`,
		memberID, eventID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load role")
		return
	}
	c.JSON(http.StatusOK, roleResponse{Role: role})
}

func eventIDFromPath(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

func memberIDFromPath(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("member_id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
