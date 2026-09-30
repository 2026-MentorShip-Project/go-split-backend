package events

import (
	"encoding/json"
	"slices"
	"testing"

	"go-split-backend/internal/rulespec"
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

func TestEmbeddedTemplateRulesAreValid(t *testing.T) {
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	for _, tpl := range tpls {
		t.Run(tpl.Label, func(t *testing.T) {
			var content struct {
				ItemTags []string `json:"item_tags"`
				CondTags []string `json:"cond_tags"`
				Rules    []struct {
					Tag    string          `json:"tag"`
					Groups json.RawMessage `json:"groups"`
					Rest   json.RawMessage `json:"rest"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(tpl.rawContent, &content); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			seen := map[string]bool{}
			for _, r := range content.Rules {
				if !slices.Contains(content.ItemTags, r.Tag) {
					t.Errorf("rule tag %q is not in item_tags", r.Tag)
				}
				if seen[r.Tag] {
					t.Errorf("duplicate rule for tag %q", r.Tag)
				}
				seen[r.Tag] = true
				if _, _, err := rulespec.NormalizeRule(r.Groups, r.Rest, content.CondTags); err != nil {
					t.Errorf("rule %q: %v", r.Tag, err)
				}
			}
		})
	}
}

func TestLoadEmbeddedTemplatesShipsCorpTrip(t *testing.T) {
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	for _, tpl := range tpls {
		if tpl.Label != "公司旅遊模板" {
			continue
		}
		got, err := summarizeTemplate(templateItem{Label: tpl.Label, Content: tpl.rawContent})
		if err != nil {
			t.Fatalf("summarizeTemplate: %v", err)
		}
		if got.RuleCount != 13 || got.ItemTagCount != 14 || got.CondTagCount != 14 {
			t.Errorf("counts = %d rules, %d item tags, %d cond tags; want 13, 14, 14",
				got.RuleCount, got.ItemTagCount, got.CondTagCount)
		}
		return
	}
	t.Fatal("公司旅遊模板 not embedded")
}
