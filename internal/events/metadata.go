package events

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// PatchEvent godoc
// @Summary Edit an active event's metadata (template is immutable)
// @Tags events
// @Accept json
// @Param id path int true "Event id"
// @Param body body metadataRequest true "Name, place and dates; omit template"
// @Success 204
// @Router /events/{id} [patch]
func (h *Handler) PatchEvent(c *gin.Context) {
	var req metadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, "invalid event metadata")
		return
	}
	if req.Template != "" {
		respondErr(c, 409, "template cannot be changed after creation")
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Name) > 120 || len(req.Place) > 200 {
		respondErr(c, 400, "invalid event metadata")
		return
	}
	starts, err := parseDate(trimmed(req.StartsAt))
	if err != nil {
		respondErr(c, 400, "invalid starts_at: "+err.Error())
		return
	}
	ends, err := parseDate(trimmed(req.EndsAt))
	if err != nil {
		respondErr(c, 400, "invalid ends_at: "+err.Error())
		return
	}
	if starts != nil && ends != nil && ends.Before(*starts) {
		respondErr(c, 400, "ends_at is before starts_at")
		return
	}
	// An omitted date keeps the stored one; an empty string clears it. A client
	// editing only the name must not wipe the schedule.
	if _, err := h.DB.Exec(c.Request.Context(), `
		UPDATE events
		   SET name      = $2,
		       place     = $3,
		       starts_at = CASE WHEN $4 THEN $5::date ELSE starts_at END,
		       ends_at   = CASE WHEN $6 THEN $7::date ELSE ends_at   END
		 WHERE id = $1`,
		eventIDFromPath(c), strings.TrimSpace(req.Name), strings.TrimSpace(req.Place),
		req.StartsAt != nil, nullableDate(formatDate(starts)),
		req.EndsAt != nil, nullableDate(formatDate(ends))); err != nil {
		respondErr(c, 500, "update event")
		return
	}
	c.Status(204)
}

type metadataRequest struct {
	Name     string  `json:"name" binding:"required,max=120"`
	Place    string  `json:"place" binding:"max=200"`
	Template string  `json:"template"`
	StartsAt *string `json:"starts_at" example:"2026-09-27"`
	EndsAt   *string `json:"ends_at" example:"2026-09-28"`
}
