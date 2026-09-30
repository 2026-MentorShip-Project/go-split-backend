package events

import "testing"

func TestSummarizeTemplateCountsOutdoorTemplate(t *testing.T) {
	t.Parallel()
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	var item templateItem
	for _, tpl := range tpls {
		if tpl.Label == "烤肉/露營模板" {
			item = templateItem{Label: tpl.Label, Description: tpl.Description, Content: tpl.rawContent}
		}
	}
	if item.Label == "" {
		t.Fatal("outdoor template not embedded")
	}

	got, err := summarizeTemplate(item)
	if err != nil {
		t.Fatalf("summarizeTemplate: %v", err)
	}
	if got.ItemTagCount != 16 || got.CondTagCount != 8 || got.RuleCount != 6 {
		t.Errorf("counts = %d item tags, %d cond tags, %d rules; want 16, 8, 6",
			got.ItemTagCount, got.CondTagCount, got.RuleCount)
	}
	if got.RuleGroupCount != 12 {
		t.Errorf("RuleGroupCount = %d; want 12", got.RuleGroupCount)
	}
}

func TestSummarizeTemplateReturnsZerosForEmptyContent(t *testing.T) {
	t.Parallel()
	got, err := summarizeTemplate(templateItem{Label: "x", Content: []byte(`{"item_tags":[],"cond_tags":[],"rules":[]}`)})
	if err != nil {
		t.Fatalf("summarizeTemplate: %v", err)
	}
	if got.ItemTagCount != 0 || got.CondTagCount != 0 || got.RuleCount != 0 || got.RuleGroupCount != 0 {
		t.Errorf("got %+v; want all counts zero", got)
	}
}

func TestSummarizeTemplateRejectsMalformedContent(t *testing.T) {
	t.Parallel()
	if _, err := summarizeTemplate(templateItem{Content: []byte(`{`)}); err == nil {
		t.Error("summarizeTemplate accepted malformed JSON")
	}
}
