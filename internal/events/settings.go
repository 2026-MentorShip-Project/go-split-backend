package events

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

func (h *Handler) registerSettingsRoutes(g *gin.RouterGroup) {
	anyRole := auth.RequireEventRole(h.DB, "host", "co", "member")
	hostOnly := auth.RequireEventRole(h.DB, "host")

	g.POST("/:id/apply-template", hostOnly, h.PostApplyTemplate)

	g.GET("/:id/tags/items", anyRole, h.GetItemTagsCatalog)
	g.GET("/:id/tags/conds", anyRole, h.GetCondTagsCatalog)
	g.GET("/:id/rules", anyRole, h.GetRules)
}

type labelsResponse struct {
	Labels []string `json:"labels"`
}

type ruleDTO struct {
	ID      int64           `json:"id"`
	ItemTag string          `json:"item_tag"`
	Groups  json.RawMessage `json:"groups" swaggertype:"array,object"`
	Rest    json.RawMessage `json:"rest,omitempty" swaggertype:"object"`
	Ordinal int             `json:"ordinal"`
}

type rulesResponse struct {
	Rules []ruleDTO `json:"rules"`
}

type applyTemplateRequest struct {
	Label string `json:"label" binding:"required,min=1,max=64"`
}

type applyTemplateResponse struct {
	Label    string    `json:"label"`
	ItemTags []string  `json:"item_tags"`
	CondTags []string  `json:"cond_tags"`
	Rules    []ruleDTO `json:"rules"`
}

// PostApplyTemplate godoc
// @Summary     Reset an event's tags and rules from a template
// @Description Host-only. Reads the template's content and replaces the
// @Description event's tag catalogs and rules in one transaction. Items
// @Description already recorded keep their tag strings unchanged even if
// @Description the new catalog no longer lists them.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int                  true "Event id"
// @Param       body body applyTemplateRequest true "Template label"
// @Success     200  {object} applyTemplateResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     404  {object} errorResponse
// @Router      /events/{id}/apply-template [post]
func (h *Handler) PostApplyTemplate(c *gin.Context) {
	var req applyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)

	content, err := loadTemplateContent(c.Request.Context(), h.DB, req.Label)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(c, http.StatusNotFound, "template not found")
			return
		}
		respondErr(c, http.StatusInternalServerError, "load template")
		return
	}

	if err := replaceEventSettings(c.Request.Context(), h.DB, eventID, content); err != nil {
		respondErr(c, http.StatusInternalServerError, "apply template")
		return
	}

	itemTags, _ := loadTagLabels(c.Request.Context(), h.DB, eventID, "event_item_tags")
	condTags, _ := loadTagLabels(c.Request.Context(), h.DB, eventID, "event_cond_tags")
	rules, _ := loadRules(c.Request.Context(), h.DB, eventID)

	c.JSON(http.StatusOK, applyTemplateResponse{
		Label:    req.Label,
		ItemTags: itemTags,
		CondTags: condTags,
		Rules:    rules,
	})
}

// GetItemTagsCatalog godoc
// @Summary     List an event's item tag catalog
// @Tags        settings
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} labelsResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/tags/items [get]
func (h *Handler) GetItemTagsCatalog(c *gin.Context) {
	labels, err := loadTagLabels(c.Request.Context(), h.DB, eventIDFromPath(c), "event_item_tags")
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list item tags")
		return
	}
	c.JSON(http.StatusOK, labelsResponse{Labels: labels})
}

// GetCondTagsCatalog godoc
// @Summary     List an event's person-condition tag catalog
// @Tags        settings
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} labelsResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/tags/conds [get]
func (h *Handler) GetCondTagsCatalog(c *gin.Context) {
	labels, err := loadTagLabels(c.Request.Context(), h.DB, eventIDFromPath(c), "event_cond_tags")
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list cond tags")
		return
	}
	c.JSON(http.StatusOK, labelsResponse{Labels: labels})
}

// GetRules godoc
// @Summary     List an event's split rules
// @Tags        settings
// @Produce     json
// @Param       id path int true "Event id"
// @Success     200 {object} rulesResponse
// @Failure     401 {object} errorResponse
// @Failure     403 {object} errorResponse
// @Router      /events/{id}/rules [get]
func (h *Handler) GetRules(c *gin.Context) {
	rules, err := loadRules(c.Request.Context(), h.DB, eventIDFromPath(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list rules")
		return
	}
	c.JSON(http.StatusOK, rulesResponse{Rules: rules})
}

type templateContent struct {
	ItemTags []string          `json:"item_tags"`
	CondTags []string          `json:"cond_tags"`
	Rules    []templateRuleRaw `json:"rules"`
}

type templateRuleRaw struct {
	Tag    string          `json:"tag"`
	Groups json.RawMessage `json:"groups"`
	Rest   json.RawMessage `json:"rest,omitempty"`
}

func loadTemplateContent(ctx context.Context, db *pgxpool.Pool, label string) (templateContent, error) {
	var body []byte
	err := db.QueryRow(ctx,
		`SELECT content::text FROM templates WHERE label = $1`, label).Scan(&body)
	if err != nil {
		return templateContent{}, err
	}
	var out templateContent
	if err := json.Unmarshal(body, &out); err != nil {
		return templateContent{}, err
	}
	return out, nil
}

func replaceEventSettings(ctx context.Context, db *pgxpool.Pool, eventID int64, content templateContent) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, tbl := range []string{"event_item_tags", "event_cond_tags", "event_rules"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+tbl+" WHERE event_id = $1", eventID); err != nil {
			return err
		}
	}
	for i, label := range content.ItemTags {
		if _, err := tx.Exec(ctx,
			`INSERT INTO event_item_tags (event_id, label, ordinal) VALUES ($1, $2, $3)`,
			eventID, label, i); err != nil {
			return err
		}
	}
	for i, label := range content.CondTags {
		if _, err := tx.Exec(ctx,
			`INSERT INTO event_cond_tags (event_id, label, ordinal) VALUES ($1, $2, $3)`,
			eventID, label, i); err != nil {
			return err
		}
	}
	for i, r := range content.Rules {
		groups := r.Groups
		if len(groups) == 0 {
			groups = json.RawMessage(`[]`)
		}
		var restArg any
		if len(r.Rest) > 0 && string(r.Rest) != "null" {
			restArg = string(r.Rest)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_rules (event_id, item_tag, groups, rest, ordinal)
			VALUES ($1, $2, $3::jsonb, $4::jsonb, $5)`,
			eventID, r.Tag, string(groups), restArg, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func loadTagLabels(ctx context.Context, db *pgxpool.Pool, eventID int64, table string) ([]string, error) {
	rows, err := db.Query(ctx,
		`SELECT label FROM `+table+` WHERE event_id = $1 ORDER BY ordinal, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func loadRules(ctx context.Context, db *pgxpool.Pool, eventID int64) ([]ruleDTO, error) {
	rows, err := db.Query(ctx, `
		SELECT id, item_tag, groups::text, COALESCE(rest::text, ''), ordinal
		  FROM event_rules
		 WHERE event_id = $1
		 ORDER BY ordinal, id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ruleDTO{}
	for rows.Next() {
		var (
			r          ruleDTO
			groupsText string
			restText   string
		)
		if err := rows.Scan(&r.ID, &r.ItemTag, &groupsText, &restText, &r.Ordinal); err != nil {
			return nil, err
		}
		r.Groups = json.RawMessage(groupsText)
		if restText != "" {
			r.Rest = json.RawMessage(restText)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
