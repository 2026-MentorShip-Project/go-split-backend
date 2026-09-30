package events

import (
	"testing"
)

func TestAllowedTemplatesMatchEmbeddedFiles(t *testing.T) {
	tpls, err := LoadEmbeddedTemplates()
	if err != nil {
		t.Fatalf("LoadEmbeddedTemplates: %v", err)
	}
	if got := len(allowedTemplates); got != len(tpls) {
		t.Fatalf("allowedTemplates has %d entries; want %d, one per file", got, len(tpls))
	}
	for _, tpl := range tpls {
		if !allowedTemplates[tpl.Label] {
			t.Errorf("allowedTemplates missing %q", tpl.Label)
		}
	}
}

func TestAllowedTemplatesIncludeCustomAndRejectUnknown(t *testing.T) {
	if !allowedTemplates["自訂"] {
		t.Error("自訂 must be allowed; the create page selects it by default")
	}
	for _, label := range []string{"not a template", "唱歌模板"} {
		if allowedTemplates[label] {
			t.Errorf("%q has no template file but is allowed", label)
		}
	}
}
