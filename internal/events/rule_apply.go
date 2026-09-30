package events

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"go-split-backend/internal/auth"
	"go-split-backend/internal/database"
	"go-split-backend/internal/ruleassist"

	"github.com/gin-gonic/gin"
)

type ruleApplyRejection struct {
	Error  string             `json:"error"`
	Issues []ruleassist.Issue `json:"issues"`
}

// PostRuleApply godoc
// @Summary     Apply a drafted rule plan in one transaction
// @Description Host-only. Send the plan from POST /rules/draft unchanged. It is
// @Description checked again against the event as it is now; if anything no
// @Description longer applies, nothing is written and 422 lists why. Otherwise
// @Description new tags, rules and member conditions are saved together.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int             true "Event id"
// @Param       body body ruleassist.Plan true "Plan returned by the draft endpoint"
// @Success     204  "no content"
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Failure     422  {object} ruleApplyRejection
// @Router      /events/{id}/rules/apply [post]
func (h *Handler) PostRuleApply(c *gin.Context) {
	var plan ruleassist.Plan
	if err := c.ShouldBindJSON(&plan); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()
	eventID := eventIDFromPath(c)

	in, err := loadDraftInput(ctx, h.DB, eventID, auth.CurrentSubject(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load event settings")
		return
	}
	checked, issues := ruleassist.Check(plan, in)
	issues = append(issues, staleOps(plan, checked)...)
	checked, issues, err = refuseLockedCreates(ctx, h.DB, eventID, checked, issues)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "check rule use")
		return
	}
	if len(issues) > 0 {
		c.JSON(http.StatusUnprocessableEntity, ruleApplyRejection{Error: "plan no longer applies", Issues: issues})
		return
	}

	if err := applyPlan(ctx, h.DB, eventID, checked); err != nil {
		if isUniqueViolation(err) {
			respondErr(c, http.StatusConflict, "plan collides with a change made since the draft")
			return
		}
		respondErr(c, http.StatusInternalServerError, "apply plan")
		return
	}
	c.Status(http.StatusNoContent)
}

// staleOps reports rules whose op Check had to correct: the host approved a
// create or a replace, and applying the other would not be what they saw.
func staleOps(requested, checked ruleassist.Plan) []ruleassist.Issue {
	asked := map[string]ruleassist.Op{}
	for _, r := range requested.Rules {
		asked[r.ItemTag] = r.Op
	}
	out := []ruleassist.Issue{}
	for _, r := range checked.Rules {
		if asked[r.ItemTag] != r.Op {
			out = append(out, ruleassist.Issue{ItemTag: r.ItemTag, Code: ruleassist.InvalidOp,
				Detail: fmt.Sprintf("the rule for %q changed since the draft; draft again", r.ItemTag)})
		}
	}
	return out
}

func applyPlan(ctx context.Context, db database.Store, eventID int64, p ruleassist.Plan) error {
	for _, label := range p.NewItemTags {
		if err := insertLabel(ctx, db, eventID, "event_item_tags", label); err != nil {
			return err
		}
	}
	for _, label := range p.NewCondTags {
		if err := insertLabel(ctx, db, eventID, "event_cond_tags", label); err != nil {
			return err
		}
	}
	for _, r := range p.Rules {
		if err := saveRule(ctx, db, eventID, r); err != nil {
			return err
		}
	}
	for _, mc := range p.MemberConds {
		if err := addMemberConds(ctx, db, eventID, mc); err != nil {
			return err
		}
	}
	return nil
}

func insertLabel(ctx context.Context, db database.Store, eventID int64, table, label string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO `+table+` (event_id, label, ordinal)
		 VALUES ($1, $2, COALESCE((SELECT MAX(ordinal) + 1 FROM `+table+` WHERE event_id = $1), 0))`,
		eventID, label)
	return err
}

func saveRule(ctx context.Context, db database.Store, eventID int64, r ruleassist.PlannedRule) error {
	groups := string(r.Groups)
	if len(r.Groups) == 0 {
		groups = `[]`
	}
	var rest any
	if len(r.Rest) > 0 && string(r.Rest) != "null" {
		rest = string(r.Rest)
	}
	if r.Op == ruleassist.Replace {
		_, err := db.Exec(ctx,
			`UPDATE event_rules SET groups = $3::jsonb, rest = $4::jsonb WHERE event_id = $1 AND item_tag = $2`,
			eventID, r.ItemTag, groups, rest)
		return err
	}
	_, err := db.Exec(ctx, `
		INSERT INTO event_rules (event_id, item_tag, groups, rest, ordinal)
		VALUES ($1, $2, $3::jsonb, $4::jsonb,
		        COALESCE((SELECT MAX(ordinal) + 1 FROM event_rules WHERE event_id = $1), 0))`,
		eventID, r.ItemTag, groups, rest)
	return err
}

func addMemberConds(ctx context.Context, db database.Store, eventID int64, mc ruleassist.MemberCond) error {
	var tags []string
	if err := db.QueryRow(ctx, "SELECT tags FROM event_members WHERE event_id = $1 AND id = $2", eventID, mc.MemberID).Scan(&tags); err != nil {
		return err
	}
	for _, t := range mc.Add {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	_, err := db.Exec(ctx, "UPDATE event_members SET tags = $3 WHERE event_id = $1 AND id = $2", eventID, mc.MemberID, tags)
	return err
}
