package events

import (
	"context"
	"encoding/json"
	"net/http"

	"go-split-backend/internal/database"

	"github.com/gin-gonic/gin"

	"go-split-backend/internal/auth"
)

// registerTemplateRoutes wires the /templates endpoint. Session-gated for
// consistency with the rest of the API; the picker is only reachable after
// login anyway.
func (h *Handler) registerTemplateRoutes(r gin.IRouter) {
	g := r.Group("/templates", auth.RequireSession(h.DB))
	g.GET("", h.GetTemplates)
	g.GET("/summary", h.GetTemplateSummary)
}

type templateItem struct {
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Soon        bool            `json:"soon"`
	Content     json.RawMessage `json:"content" swaggertype:"object"`
}

type templatesResponse struct {
	Templates []templateItem `json:"templates"`
}

// GetTemplates godoc
// @Summary     List every template in the catalog
// @Description Returns each seeded template with its label, description, and
// @Description full JSON content (item_tags, cond_tags, rules). The client
// @Description caches this for the create-event picker and reuses the
// @Description content when applying a template to a new event's settings.
// @Tags        templates
// @Produce     json
// @Success     200 {object} templatesResponse
// @Failure     401 {object} errorResponse
// @Router      /templates [get]
func (h *Handler) GetTemplates(c *gin.Context) {
	rows, err := loadAllTemplates(c.Request.Context(), h.DB)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list templates")
		return
	}
	c.JSON(http.StatusOK, templatesResponse{Templates: rows})
}

type templateSummaryResponse struct {
	Label          string `json:"label"`
	Description    string `json:"description"`
	Soon           bool   `json:"soon"`
	ItemTagCount   int    `json:"item_tag_count"`
	CondTagCount   int    `json:"cond_tag_count"`
	RuleCount      int    `json:"rule_count"`
	RuleGroupCount int    `json:"rule_group_count"`
}

// GetTemplateSummary godoc
// @Summary     Summarize one template
// @Description Returns counts of item tags, condition tags, rules, and rule
// @Description groups for the template with the given label. The label is a
// @Description query parameter because labels can contain "/".
// @Tags        templates
// @Produce     json
// @Param       label query string true "Template label"
// @Success     200 {object} templateSummaryResponse
// @Failure     400 {object} errorResponse
// @Failure     401 {object} errorResponse
// @Failure     404 {object} errorResponse
// @Router      /templates/summary [get]
func (h *Handler) GetTemplateSummary(c *gin.Context) {
	label := c.Query("label")
	if label == "" {
		respondErr(c, http.StatusBadRequest, "label is required")
		return
	}
	rows, err := loadAllTemplates(c.Request.Context(), h.DB)
	if err != nil {
		respondErr(c, http.StatusInternalServerError, "list templates")
		return
	}
	for _, it := range rows {
		if it.Label != label {
			continue
		}
		summary, err := summarizeTemplate(it)
		if err != nil {
			respondErr(c, http.StatusInternalServerError, "parse template content")
			return
		}
		c.JSON(http.StatusOK, summary)
		return
	}
	respondErr(c, http.StatusNotFound, "template not found")
}

func summarizeTemplate(it templateItem) (templateSummaryResponse, error) {
	var content struct {
		ItemTags []json.RawMessage `json:"item_tags"`
		CondTags []json.RawMessage `json:"cond_tags"`
		Rules    []struct {
			Groups []json.RawMessage `json:"groups"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(it.Content, &content); err != nil {
		return templateSummaryResponse{}, err
	}
	groups := 0
	for _, r := range content.Rules {
		groups += len(r.Groups)
	}
	return templateSummaryResponse{
		Label:          it.Label,
		Description:    it.Description,
		Soon:           it.Soon,
		ItemTagCount:   len(content.ItemTags),
		CondTagCount:   len(content.CondTags),
		RuleCount:      len(content.Rules),
		RuleGroupCount: groups,
	}, nil
}

func loadAllTemplates(ctx context.Context, db database.Store) ([]templateItem, error) {
	rows, err := db.Query(ctx, `
		SELECT label, description, content::text
		  FROM templates
		 ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []templateItem{}
	for rows.Next() {
		var (
			it      templateItem
			content string
		)
		if err := rows.Scan(&it.Label, &it.Description, &content); err != nil {
			return nil, err
		}
		it.Content = json.RawMessage(content)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
