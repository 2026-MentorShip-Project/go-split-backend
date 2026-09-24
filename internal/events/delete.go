package events

import "github.com/gin-gonic/gin"

// DeleteEvent godoc
// @Summary Delete an active event with all its members, items, and settings
// @Description Host only. Settled and archived events are read-only and return 409.
// @Tags events
// @Param id path int true "Event id"
// @Success 204
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Router /events/{id} [delete]
func (h *Handler) DeleteEvent(c *gin.Context) {
	if _, err := h.DB.Exec(c.Request.Context(), "DELETE FROM events WHERE id=$1", eventIDFromPath(c)); err != nil {
		respondErr(c, 500, "delete event")
		return
	}
	c.Status(204)
}
