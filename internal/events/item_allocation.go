package events

import (
	"go-split-backend/internal/auth"

	"github.com/gin-gonic/gin"
)

func (h *Handler) populateItemAllocation(c *gin.Context, item *itemDTO) error {
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	result, err := computeEventShares(ctx, h.DB, id)
	if err != nil {
		return err
	}
	var archived bool
	if err = h.DB.QueryRow(ctx, "SELECT archived FROM events WHERE id=$1", id).Scan(&archived); err != nil {
		return err
	}
	filter := int64(0)
	if auth.EventRole(c) != "host" && !archived {
		filter = auth.EventMemberID(c)
	}
	shares := sharesFromResult(result, filter)
	for i := range item.Details {
		for _, d := range shares.PerDetail {
			if d.DetailID == item.Details[i].ID {
				allocation := d.SplitResult
				item.Details[i].Allocation = &allocation
				break
			}
		}
	}
	return nil
}
