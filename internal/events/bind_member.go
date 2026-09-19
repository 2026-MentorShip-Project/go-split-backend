package events

import (
	"errors"
	"go-split-backend/internal/auth"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type bindMemberRequest struct {
	JoinedMemberID int64 `json:"joined_member_id" binding:"required,gt=0"`
}

// BindMember godoc
// @Summary Bind an already-joined identity to a host-created placeholder
// @Description Host only, active events only. Keeps placeholder ID, display, role, tags, note and split order. Moves expense references and deletes the duplicate joined membership. Conflicting custom amounts return 409. Splits recalculate with one fewer member.
// @Tags members
// @Accept json
// @Produce json
// @Param id path int true "Event id"
// @Param member_id path int true "Placeholder member id to keep"
// @Param body body bindMemberRequest true "Joined member to bind"
// @Success 200 {object} memberDTO
// @Failure 409 {object} errorResponse
// @Router /events/{id}/members/{member_id}/bind [post]
func (h *Handler) BindMember(c *gin.Context) {
	target, ok := memberIDFromPath(c)
	var req bindMemberRequest
	if !ok || c.ShouldBindJSON(&req) != nil || target == req.JoinedMemberID {
		respondErr(c, 400, "different placeholder and joined member ids are required")
		return
	}
	ctx := c.Request.Context()
	eventID := eventIDFromPath(c)
	var virtual bool
	if err := h.DB.QueryRow(ctx, "SELECT virtual FROM event_members WHERE event_id=$1 AND id=$2", eventID, target).Scan(&virtual); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, 404, "placeholder not found")
		} else {
			respondErr(c, 500, "load placeholder")
		}
		return
	}
	if !virtual {
		respondErr(c, 409, "target member is already bound to an identity")
		return
	}
	var account, guest *int64
	var role string
	err := h.DB.QueryRow(ctx, "SELECT account_id,guest_id,role::text FROM event_members WHERE event_id=$1 AND id=$2", eventID, req.JoinedMemberID).Scan(&account, &guest, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, 404, "joined member not found in this event")
		return
	}
	if err != nil {
		respondErr(c, 500, "load joined member")
		return
	}
	if role == "host" || (account == nil && guest == nil) {
		respondErr(c, 409, "source must be a joined non-host member")
		return
	}
	var conflicts bool
	if err = h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM item_details d JOIN items i ON i.id=d.item_id WHERE i.event_id=$1 AND d.custom_shares ? $2::text AND d.custom_shares ? $3::text)`, eventID, strconv.FormatInt(req.JoinedMemberID, 10), strconv.FormatInt(target, 10)).Scan(&conflicts); err != nil {
		respondErr(c, 500, "check custom allocations")
		return
	}
	if conflicts {
		respondErr(c, 409, "both members have custom amounts on the same detail; reconcile them before binding")
		return
	}
	if err = h.moveJoinedMember(c, eventID, req.JoinedMemberID, target, account, guest); err != nil {
		respondErr(c, 500, "bind joined member")
		return
	}
	result, err := computeEventShares(ctx, h.DB, eventID)
	if err != nil {
		respondErr(c, 500, "validate bound allocations")
		return
	}
	if issues := splitIssues(result); len(issues) > 0 {
		c.JSON(422, validationResponse{Error: "binding would leave invalid allocations", Details: issues})
		return
	}
	members, err := loadMembers(ctx, h.DB, eventID, auth.CurrentSubject(c))
	if err != nil {
		respondErr(c, 500, "reload member")
		return
	}
	for _, m := range members {
		if m.ID == target {
			c.JSON(200, m)
			return
		}
	}
	respondErr(c, 500, "bound member missing")
}

func (h *Handler) moveJoinedMember(c *gin.Context, eventID, source, target int64, account, guest *int64) error {
	ctx := c.Request.Context()
	if _, err := h.DB.Exec(ctx, `UPDATE items SET payer_member_id=CASE WHEN payer_member_id=$2 THEN $3 ELSE payer_member_id END,author_member_id=CASE WHEN author_member_id=$2 THEN $3 ELSE author_member_id END WHERE event_id=$1 AND (payer_member_id=$2 OR author_member_id=$2)`, eventID, source, target); err != nil {
		return err
	}
	if _, err := h.DB.Exec(ctx, `UPDATE item_details d SET
 custom_shares=CASE WHEN custom_shares ? $2 THEN (custom_shares-$2)||jsonb_build_object($3::text,custom_shares->$2) ELSE custom_shares END,
 manual_member_ids=CASE WHEN manual_member_ids IS NULL THEN NULL ELSE ARRAY(SELECT mid FROM unnest(array_replace(manual_member_ids,$4,$5)) WITH ORDINALITY a(mid,ord) GROUP BY mid ORDER BY min(ord)) END
 WHERE item_id IN (SELECT id FROM items WHERE event_id=$1) AND (custom_shares ? $2 OR $4=ANY(manual_member_ids))`, eventID, strconv.FormatInt(source, 10), strconv.FormatInt(target, 10), source, target); err != nil {
		return err
	}
	if _, err := h.DB.Exec(ctx, "DELETE FROM event_members WHERE event_id=$1 AND id=$2", eventID, source); err != nil {
		return err
	}
	_, err := h.DB.Exec(ctx, "UPDATE event_members SET account_id=$3,guest_id=$4 WHERE event_id=$1 AND id=$2", eventID, target, account, guest)
	return err
}
