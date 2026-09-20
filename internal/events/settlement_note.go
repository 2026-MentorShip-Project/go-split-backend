package events

import "github.com/gin-gonic/gin"

type settlementNoteRequest struct {
	Note *string `json:"note" binding:"required,max=2000"`
}

// PatchSettlementNote godoc
// @Summary Save the settlement output message before settlement
// @Description Host only. Empty string clears the note. Read transfer_note from GET /events/{id}.
// @Tags settlement
// @Accept json
// @Param id path int true "Event id"
// @Param body body settlementNoteRequest true "Settlement message"
// @Success 204
// @Failure 409 {object} errorResponse
// @Router /events/{id}/settlement-note [patch]
func (h *Handler) PatchSettlementNote(c *gin.Context) {
	var req settlementNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, "note is required, maximum 2000 characters")
		return
	}
	if _, err := h.DB.Exec(c.Request.Context(), "UPDATE events SET transfer_note=$2 WHERE id=$1", eventIDFromPath(c), *req.Note); err != nil {
		respondErr(c, 500, "save settlement note")
		return
	}
	c.Status(204)
}
