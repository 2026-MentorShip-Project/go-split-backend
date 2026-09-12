package events

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
	g.POST("/:id/tags/items", hostOnly, h.PostItemTag)
	g.DELETE("/:id/tags/items/:label", hostOnly, h.DeleteItemTag)

	g.GET("/:id/tags/conds", anyRole, h.GetCondTagsCatalog)
	g.POST("/:id/tags/conds", hostOnly, h.PostCondTag)
	g.DELETE("/:id/tags/conds/:label", hostOnly, h.DeleteCondTag)

	g.GET("/:id/rules", anyRole, h.GetRules)
	g.POST("/:id/rules", hostOnly, h.PostRule)
	g.PATCH("/:id/rules/:rule_id", hostOnly, h.PatchRule)
	g.DELETE("/:id/rules/:rule_id", hostOnly, h.DeleteRule)
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

type addLabelRequest struct {
	Label string `json:"label" binding:"required,min=1,max=64"`
}

// PostItemTag godoc
// @Summary     Add an item tag to an event's catalog
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int             true "Event id"
// @Param       body body addLabelRequest true "Label to add"
// @Success     201  {object} labelsResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /events/{id}/tags/items [post]
func (h *Handler) PostItemTag(c *gin.Context) {
	h.appendTag(c, "event_item_tags")
}

// PostCondTag godoc
// @Summary     Add a condition tag to an event's catalog
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int             true "Event id"
// @Param       body body addLabelRequest true "Label to add"
// @Success     201  {object} labelsResponse
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /events/{id}/tags/conds [post]
func (h *Handler) PostCondTag(c *gin.Context) {
	h.appendTag(c, "event_cond_tags")
}

// DeleteItemTag godoc
// @Summary     Remove an item tag from an event's catalog
// @Tags        settings
// @Produce     json
// @Param       id    path int    true "Event id"
// @Param       label path string true "Tag label"
// @Success     204   "no content"
// @Failure     401   {object} errorResponse
// @Failure     403   {object} errorResponse
// @Failure     404   {object} errorResponse
// @Router      /events/{id}/tags/items/{label} [delete]
func (h *Handler) DeleteItemTag(c *gin.Context) {
	h.removeTag(c, "event_item_tags")
}

// DeleteCondTag godoc
// @Summary     Remove a condition tag from an event's catalog
// @Tags        settings
// @Produce     json
// @Param       id    path int    true "Event id"
// @Param       label path string true "Tag label"
// @Success     204   "no content"
// @Failure     401   {object} errorResponse
// @Failure     403   {object} errorResponse
// @Failure     404   {object} errorResponse
// @Router      /events/{id}/tags/conds/{label} [delete]
func (h *Handler) DeleteCondTag(c *gin.Context) {
	h.removeTag(c, "event_cond_tags")
}

func (h *Handler) appendTag(c *gin.Context, table string) {
	var req addLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	ctx := c.Request.Context()

	_, err := h.DB.Exec(ctx,
		`INSERT INTO `+table+` (event_id, label, ordinal)
		 VALUES ($1, $2, COALESCE((SELECT MAX(ordinal) + 1 FROM `+table+` WHERE event_id = $1), 0))`,
		eventID, req.Label)
	if err != nil {
		if isUniqueViolation(err) {
			respondErr(c, http.StatusConflict, "label already exists")
			return
		}
		respondErr(c, http.StatusInternalServerError, "add tag")
		return
	}
	labels, err := loadTagLabels(ctx, h.DB, eventID, table)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "reload tags")
		return
	}
	c.JSON(http.StatusCreated, labelsResponse{Labels: labels})
}

func (h *Handler) removeTag(c *gin.Context, table string) {
	eventID := eventIDFromPath(c)
	label := c.Param("label")
	if label == "" {
		respondErr(c, http.StatusBadRequest, "label required")
		return
	}
	tag, err := h.DB.Exec(c.Request.Context(),
		`DELETE FROM `+table+` WHERE event_id = $1 AND label = $2`,
		eventID, label)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "delete tag")
		return
	}
	if tag.RowsAffected() == 0 {
		respondErr(c, http.StatusNotFound, "tag not found")
		return
	}
	c.Status(http.StatusNoContent)
}

type ruleBodyRequest struct {
	ItemTag string          `json:"item_tag"        binding:"required,min=1,max=64"`
	Groups  json.RawMessage `json:"groups"          swaggertype:"array,object"`
	Rest    json.RawMessage `json:"rest,omitempty"  swaggertype:"object"`
}

type ruleUpdateRequest struct {
	Groups *json.RawMessage `json:"groups,omitempty" swaggertype:"array,object"`
	Rest   *json.RawMessage `json:"rest,omitempty"   swaggertype:"object"`
}

// PostRule godoc
// @Summary     Add a rule to an event
// @Description Host-only. Appends to the end of the rule list. item_tag
// @Description must be unique per event; a duplicate returns 409.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id   path int              true "Event id"
// @Param       body body ruleBodyRequest true "New rule"
// @Success     201  {object} ruleDTO
// @Failure     400  {object} errorResponse
// @Failure     401  {object} errorResponse
// @Failure     403  {object} errorResponse
// @Failure     409  {object} errorResponse
// @Router      /events/{id}/rules [post]
func (h *Handler) PostRule(c *gin.Context) {
	var req ruleBodyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)
	groups := req.Groups
	if len(groups) == 0 {
		groups = json.RawMessage(`[]`)
	}
	var restArg any
	if len(req.Rest) > 0 && string(req.Rest) != "null" {
		restArg = string(req.Rest)
	}

	var r ruleDTO
	var restText string
	err := h.DB.QueryRow(c.Request.Context(), `
		INSERT INTO event_rules (event_id, item_tag, groups, rest, ordinal)
		VALUES ($1, $2, $3::jsonb, $4::jsonb,
		        COALESCE((SELECT MAX(ordinal) + 1 FROM event_rules WHERE event_id = $1), 0))
		RETURNING id, item_tag, groups::text, COALESCE(rest::text, ''), ordinal`,
		eventID, req.ItemTag, string(groups), restArg,
	).Scan(&r.ID, &r.ItemTag, (*rawText)(&r.Groups), &restText, &r.Ordinal)
	if err != nil {
		if isUniqueViolation(err) {
			respondErr(c, http.StatusConflict, "a rule for this item tag already exists")
			return
		}
		respondErr(c, http.StatusInternalServerError, "create rule")
		return
	}
	if restText != "" {
		r.Rest = json.RawMessage(restText)
	}
	c.JSON(http.StatusCreated, r)
}

// PatchRule godoc
// @Summary     Update a rule's groups or rest bucket
// @Description Host-only. Partial update — item_tag itself is fixed after
// @Description creation. Pass null explicitly on rest to clear the
// @Description fallback and fall back to weight 1.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       id      path int               true "Event id"
// @Param       rule_id path int               true "Rule id"
// @Param       body    body ruleUpdateRequest true "Fields to change"
// @Success     200     {object} ruleDTO
// @Failure     400     {object} errorResponse
// @Failure     401     {object} errorResponse
// @Failure     403     {object} errorResponse
// @Failure     404     {object} errorResponse
// @Router      /events/{id}/rules/{rule_id} [patch]
func (h *Handler) PatchRule(c *gin.Context) {
	ruleID, ok := ruleIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid rule id")
		return
	}
	var req ruleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	eventID := eventIDFromPath(c)

	var (
		groupsArg any
		restArg   any
		restClear bool
	)
	if req.Groups != nil {
		groupsArg = string(*req.Groups)
	}
	if req.Rest != nil {
		if string(*req.Rest) == "null" {
			restClear = true
		} else {
			restArg = string(*req.Rest)
		}
	}

	var (
		r        ruleDTO
		restText string
	)
	err := h.DB.QueryRow(c.Request.Context(), `
		UPDATE event_rules
		   SET groups = COALESCE($1::jsonb, groups),
		       rest   = CASE
		                  WHEN $2 THEN NULL
		                  ELSE COALESCE($3::jsonb, rest)
		                END
		 WHERE id = $4 AND event_id = $5
	 RETURNING id, item_tag, groups::text, COALESCE(rest::text, ''), ordinal`,
		groupsArg, restClear, restArg, ruleID, eventID,
	).Scan(&r.ID, &r.ItemTag, (*rawText)(&r.Groups), &restText, &r.Ordinal)
	if errors.Is(err, pgx.ErrNoRows) {
		respondErr(c, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "update rule")
		return
	}
	if restText != "" {
		r.Rest = json.RawMessage(restText)
	}
	c.JSON(http.StatusOK, r)
}

// DeleteRule godoc
// @Summary     Delete a rule
// @Tags        settings
// @Produce     json
// @Param       id      path int true "Event id"
// @Param       rule_id path int true "Rule id"
// @Success     204     "no content"
// @Failure     400     {object} errorResponse
// @Failure     401     {object} errorResponse
// @Failure     403     {object} errorResponse
// @Failure     404     {object} errorResponse
// @Router      /events/{id}/rules/{rule_id} [delete]
func (h *Handler) DeleteRule(c *gin.Context) {
	ruleID, ok := ruleIDFromPath(c)
	if !ok {
		respondErr(c, http.StatusBadRequest, "invalid rule id")
		return
	}
	tag, err := h.DB.Exec(c.Request.Context(),
		`DELETE FROM event_rules WHERE id = $1 AND event_id = $2`,
		ruleID, eventIDFromPath(c))
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "delete rule")
		return
	}
	if tag.RowsAffected() == 0 {
		respondErr(c, http.StatusNotFound, "rule not found")
		return
	}
	c.Status(http.StatusNoContent)
}

func ruleIDFromPath(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// rawText scans a text-returning column straight into a json.RawMessage.
type rawText json.RawMessage

func (r *rawText) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*r = rawText(v)
	case []byte:
		*r = append((*r)[:0], v...)
	case nil:
		*r = nil
	default:
		return errors.New("rawText: unsupported source type")
	}
	return nil
}
