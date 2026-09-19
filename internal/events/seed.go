package events

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go-split-backend/internal/database"
)

//go:embed templates/*.json
var templatesFS embed.FS

// templateFile mirrors the on-disk JSON shape. Content stays as
// json.RawMessage so it can round-trip into the templates.content JSONB
// column unchanged.
type templateFile struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	// Content is the entire file (including label/description). Storing the
	// raw JSON avoids re-marshalling and preserves any fields we do not
	// promote to their own column.
	rawContent json.RawMessage
}

// LoadEmbeddedTemplates parses every templates/*.json file into templateFile
// values. Filenames are sorted so ordering is deterministic across runs.
func LoadEmbeddedTemplates() ([]templateFile, error) {
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]templateFile, 0, len(names))
	for _, name := range names {
		body, err := fs.ReadFile(templatesFS, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var meta struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if meta.Label == "" {
			return nil, fmt.Errorf("%s missing label", name)
		}
		out = append(out, templateFile{
			Label:       meta.Label,
			Description: meta.Description,
			rawContent:  body,
		})
	}
	return out, nil
}

// SeedTemplates upserts every embedded template into the templates table.
// Idempotent: a re-run replaces content and description in place. Returns
// the labels written in order.
func SeedTemplates(ctx context.Context, db database.Store) ([]string, error) {
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(tpls))
	for _, t := range tpls {
		if _, err := db.Exec(ctx, `
			INSERT INTO templates (label, description, content)
			VALUES ($1, $2, $3::jsonb)
			ON CONFLICT (label) DO UPDATE
			   SET description = EXCLUDED.description,
			       content     = EXCLUDED.content,
			       updated_at  = NOW()`,
			t.Label, t.Description, string(t.rawContent)); err != nil {
			return nil, fmt.Errorf("upsert template %q: %w", t.Label, err)
		}
		labels = append(labels, t.Label)
	}
	return labels, nil
}
