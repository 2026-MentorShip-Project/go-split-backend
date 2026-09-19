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
// @Param body body metadataRequest true "Name and place; omit template"
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
	if _, err := h.DB.Exec(c.Request.Context(), "UPDATE events SET name=$2,place=$3 WHERE id=$1", eventIDFromPath(c), strings.TrimSpace(req.Name), strings.TrimSpace(req.Place)); err != nil {
		respondErr(c, 500, "update event")
		return
	}
	c.Status(204)
}

type metadataRequest struct {
	Name     string `json:"name" binding:"required,max=120"`
	Place    string `json:"place" binding:"max=200"`
	Template string `json:"template"`
}
