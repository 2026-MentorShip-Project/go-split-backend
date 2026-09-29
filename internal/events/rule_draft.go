package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go-split-backend/internal/auth"
	"go-split-backend/internal/database"
	"go-split-backend/internal/ruleassist"
	"go-split-backend/internal/rulespec"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// ruleLock marks a planned rule for an item tag that expenses already use;
// POST /rules refuses those, so the draft reports it instead of promising it.
const ruleLock rulespec.Code = "rule-lock"

// registerRuleDraftRoutes sits outside eventTransaction: a POST there holds
// the event row FOR UPDATE, and a model call must not block every write.
func (h *Handler) registerRuleDraftRoutes(r gin.IRouter) {
	g := r.Group("/events", auth.RequireSession(h.DB))
	g.POST("/:id/rules/draft", auth.RequireEventRole(h.DB, "host"), h.PostRuleDraft)
}

type ruleDraftRequest struct {
	Text string `json:"text" binding:"required,min=1,max=500"`
}

type ruleDraftResponse struct {
	Plan   ruleassist.Plan    `json:"plan"`
	Issues []ruleassist.Issue `json:"issues"`
}

// PostRuleDraft godoc
// @Summary     Draft split rules from a plain-language description
// @Description Host-only and read-only: nothing is saved. The generated plan is
// @Description validated against the event and returned normalized, with every
// @Description rule that cannot be applied moved to issues. Apply the plan
// @Description through the ordinary tag, rule and member endpoints.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int              true "Event id"
// @Param       body body ruleDraftRequest true "Description of how to split"
// @Success     200  {object} ruleDraftResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Failure     502  {object} errorResponse
// @Failure     503  {object} errorResponse
// @Router      /events/{id}/rules/draft [post]
func (h *Handler) PostRuleDraft(c *gin.Context) {
	if h.Drafter == nil {
		respondErr(c, http.StatusServiceUnavailable, "rule drafting is not configured")
		return
	}
	var req ruleDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()
	eventID := eventIDFromPath(c)

	var settled, archived bool
	if err := h.DB.QueryRow(ctx, "SELECT settled, archived FROM events WHERE id=$1", eventID).Scan(&settled, &archived); err != nil {
		respondErr(c, http.StatusInternalServerError, "load event")
		return
	}
	if settled || archived {
		respondErr(c, http.StatusConflict, "event is read-only")
		return
	}

	in, err := loadDraftInput(ctx, h.DB, eventID, auth.CurrentSubject(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "load event settings")
		return
	}
	in.Text = req.Text

	plan, err := h.Drafter.Draft(ctx, in)
	switch {
	case errors.Is(err, ruleassist.ErrNotConfigured):
		respondErr(c, http.StatusServiceUnavailable, "rule drafting is not configured")
		return
	case err != nil:
		log.WithContext(ctx).WithError(err).WithField("event_id", eventID).Error("rule draft failed")
		respondErr(c, http.StatusBadGateway, "rule drafting failed")
		return
	}

	plan, issues := ruleassist.Check(plan, in)
	plan, issues, err = refuseLockedCreates(ctx, h.DB, eventID, plan, issues)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "check rule use")
		return
	}
	c.JSON(http.StatusOK, ruleDraftResponse{Plan: plan, Issues: issues})
}

func loadDraftInput(ctx context.Context, db database.Store, eventID int64, sub auth.Subject) (ruleassist.Input, error) {
	items, err := loadTagLabels(ctx, db, eventID, "event_item_tags")
	if err != nil {
		return ruleassist.Input{}, err
	}
	conds, err := loadTagLabels(ctx, db, eventID, "event_cond_tags")
	if err != nil {
		return ruleassist.Input{}, err
	}
	rules, err := loadRules(ctx, db, eventID)
	if err != nil {
		return ruleassist.Input{}, err
	}
	members, err := loadMembers(ctx, db, eventID, sub)
	if err != nil {
		return ruleassist.Input{}, err
	}

	in := ruleassist.Input{
		ItemTags: items,
		CondTags: conds,
		Rules:    make([]ruleassist.ExistingRule, 0, len(rules)),
		Members:  make([]ruleassist.Member, 0, len(members)),
	}
	for _, r := range rules {
		in.Rules = append(in.Rules, ruleassist.ExistingRule{ItemTag: r.ItemTag, Groups: r.Groups, Rest: r.Rest})
	}
	for _, m := range members {
		in.Members = append(in.Members, ruleassist.Member{ID: m.ID, Display: m.Display, Conds: m.Tags})
	}
	return in, nil
}

func refuseLockedCreates(ctx context.Context, db database.Store, eventID int64, plan ruleassist.Plan, issues []ruleassist.Issue) (ruleassist.Plan, []ruleassist.Issue, error) {
	kept := make([]ruleassist.PlannedRule, 0, len(plan.Rules))
	for _, rule := range plan.Rules {
		if rule.Op != ruleassist.Create {
			kept = append(kept, rule)
			continue
		}
		var used int
		err := db.QueryRow(ctx, "SELECT count(*) FROM item_details d JOIN items i ON i.id=d.item_id WHERE i.event_id=$1 AND d.tag=$2", eventID, rule.ItemTag).Scan(&used)
		if err != nil {
			return plan, issues, err
		}
		if used > 0 {
			issues = append(issues, ruleassist.Issue{ItemTag: rule.ItemTag, Code: ruleLock,
				Detail: fmt.Sprintf("%d expense lines already use %q", used, rule.ItemTag)})
			continue
		}
		kept = append(kept, rule)
	}
	plan.Rules = kept
	return plan, issues, nil
}
