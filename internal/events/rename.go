package events

import (
	"context"
	"encoding/json"
	"go-split-backend/internal/database"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// RenameItemTag godoc
// @Summary Rename an item tag and its rule/detail references atomically
// @Tags settings
// @Accept json
// @Param id path int true "Event id"
// @Param label path string true "Current label"
// @Param body body addLabelRequest true "New label"
// @Success 204
// @Router /events/{id}/tags/items/{label} [patch]
func (h *Handler) RenameItemTag(c *gin.Context) { h.renameTag(c, true) }

// RenameCondTag godoc
// @Summary Rename a condition and its member/rule references atomically
// @Tags settings
// @Accept json
// @Param id path int true "Event id"
// @Param label path string true "Current label"
// @Param body body addLabelRequest true "New label"
// @Success 204
// @Router /events/{id}/tags/conds/{label} [patch]
func (h *Handler) RenameCondTag(c *gin.Context) { h.renameTag(c, false) }
func (h *Handler) renameTag(c *gin.Context, item bool) {
	var req addLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, "invalid label")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" {
		respondErr(c, 400, "label required")
		return
	}
	ctx := c.Request.Context()
	id := eventIDFromPath(c)
	old := c.Param("label")
	table := "event_cond_tags"
	if item {
		table = "event_item_tags"
	}
	tag, err := h.DB.Exec(ctx, "UPDATE "+table+" SET label=$3 WHERE event_id=$1 AND label=$2", id, old, req.Label)
	if err != nil {
		if isUniqueViolation(err) {
			respondErr(c, 409, "label already exists")
		} else {
			respondErr(c, 500, "rename label")
		}
		return
	}
	if tag.RowsAffected() == 0 {
		respondErr(c, 404, "label not found")
		return
	}
	if item {
		_, err = h.DB.Exec(ctx, "UPDATE event_rules SET item_tag=$3 WHERE event_id=$1 AND item_tag=$2", id, old, req.Label)
		if err == nil {
			_, err = h.DB.Exec(ctx, "UPDATE item_details SET tag=$3 WHERE item_id IN (SELECT id FROM items WHERE event_id=$1) AND tag=$2", id, old, req.Label)
		}
	} else {
		_, err = h.DB.Exec(ctx, "UPDATE event_members SET tags=array_replace(tags,$2,$3) WHERE event_id=$1 AND $2=ANY(tags)", id, old, req.Label)
		if err == nil {
			err = rewriteConditionReferences(ctx, h.DB, id, old, req.Label)
		}
	}
	if err != nil {
		respondErr(c, 500, "rename references")
		return
	}
	c.Status(204)
}

// rewriteConditionReferences renames old to replacement in every rule group,
// or removes it when replacement is empty, dropping groups left with no
// conditions. Only the conds of affected rules change; other fields are kept
// as stored.
func rewriteConditionReferences(ctx context.Context, db database.Store, id int64, old, replacement string) error {
	rules, err := loadRules(ctx, db, id)
	if err != nil {
		return err
	}
	for _, r := range rules {
		var groups []map[string]json.RawMessage
		if err = json.Unmarshal(r.Groups, &groups); err != nil {
			return err
		}
		out := make([]map[string]json.RawMessage, 0, len(groups))
		changed := false
		for _, g := range groups {
			var conds []string
			if err = json.Unmarshal(g["conds"], &conds); err != nil {
				return err
			}
			i := slices.Index(conds, old)
			if i < 0 {
				out = append(out, g)
				continue
			}
			changed = true
			if replacement == "" {
				conds = slices.Delete(conds, i, i+1)
			} else {
				conds[i] = replacement
			}
			if len(conds) == 0 {
				continue
			}
			if g["conds"], err = json.Marshal(conds); err != nil {
				return err
			}
			out = append(out, g)
		}
		if !changed {
			continue
		}
		raw, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if _, err = db.Exec(ctx, "UPDATE event_rules SET groups=$2 WHERE id=$1", r.ID, raw); err != nil {
			return err
		}
	}
	return nil
}
