package events

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-split-backend/internal/auth"
)

// registerTemplateRoutes wires the /templates endpoint. Session-gated for
// consistency with the rest of the API; the picker is only reachable after
// login anyway.
func (h *Handler) registerTemplateRoutes(r gin.IRouter) {
	g := r.Group("/templates", auth.RequireSession(h.DB))
	g.GET("", h.GetTemplates)
}

type templateItem struct {
	Label       string          `json:"label"`
	Description string          `json:"description"`
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

func loadAllTemplates(ctx context.Context, db *pgxpool.Pool) ([]templateItem, error) {
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
	return out, rows.Err()
}
