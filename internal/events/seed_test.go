package events

import (
	"encoding/json"
	"testing"
)

func TestLoadEmbeddedTemplatesShipsOutdoor(t *testing.T) {
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	if len(tpls) == 0 {
		t.Fatal("no templates embedded")
	}

	var found *templateFile
	for i := range tpls {
		if tpls[i].Label == "烤肉/露營模板" {
			found = &tpls[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected label %q; got %d templates without it", "烤肉/露營模板", len(tpls))
	}
	if found.Description == "" {
		t.Fatal("outdoor template missing description")
	}

	// Sanity-check the JSON shape the split engine will eventually read.
	var content struct {
		Rules    []map[string]any `json:"rules"`
		ItemTags []string         `json:"item_tags"`
		CondTags []string         `json:"cond_tags"`
	}
	if err := json.Unmarshal(found.rawContent, &content); err != nil {
		t.Fatalf("unmarshal outdoor content: %v", err)
	}
	if got := len(content.Rules); got != 6 {
		t.Errorf("outdoor template has %d rules; want 6", got)
	}
	if got := len(content.ItemTags); got != 16 {
		t.Errorf("outdoor template has %d item_tags; want 16", got)
	}
	if got := len(content.CondTags); got != 8 {
		t.Errorf("outdoor template has %d cond_tags; want 8", got)
	}
}

func TestTemplateLabelsMatchAllowedList(t *testing.T) {
	// Every embedded template must be one the create endpoint accepts, or
	// creation-time template lookups would 400.
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	for _, tpl := range tpls {
		if !allowedTemplates[tpl.Label] {
			t.Errorf("embedded template %q is not in allowedTemplates", tpl.Label)
		}
	}
}
