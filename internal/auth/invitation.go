package auth

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type invitationResponse struct {
	EventID  int64    `json:"event_id"`
	Name     string   `json:"name"`
	CondTags []string `json:"cond_tags"`
}

// GetInvitation godoc
// @Summary Get an active invitation's name and available join conditions
// @Description Public lookup by invitation code. Does not expose members or expenses.
// @Tags auth
// @Produce json
// @Param code path string true "Invitation code"
// @Success 200 {object} invitationResponse
// @Failure 404 {object} errorResponse
// @Failure 410 {object} errorResponse
// @Router /auth/invite/{code} [get]
func (h *Handler) GetInvitation(c *gin.Context) {
	var out invitationResponse
	var closed bool
	err := h.DB.QueryRow(c.Request.Context(), `SELECT e.id,e.name,e.settled OR e.archived,
 ARRAY(SELECT label FROM event_cond_tags WHERE event_id=e.id ORDER BY ordinal,id)
 FROM events e JOIN invitations i ON i.event_id=e.id WHERE i.code=$1`, c.Param("code")).Scan(&out.EventID, &out.Name, &closed, &out.CondTags)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, 404, "invitation not found")
		return
	}
	if err != nil {
		respondErr(c, 500, "load invitation")
		return
	}
	if closed {
		respondErr(c, 410, "invitation expired")
		return
	}
	c.JSON(200, out)
}
