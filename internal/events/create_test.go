package events

import (
	"testing"
)

func TestAllowedTemplatesMatchesPrototype(t *testing.T) {
	// The list is closed on purpose; new labels are a product decision.
	want := []string{"自訂", "烤肉/露營模板", "聚餐模板", "唱歌模板", "國內旅遊模板", "出國旅遊模板", "社團活動模板", "公司旅遊模板"}
	if got := len(allowedTemplates); got != len(want) {
		t.Fatalf("allowedTemplates has %d entries; want %d", got, len(want))
	}
	for _, tpl := range want {
		if !allowedTemplates[tpl] {
			t.Fatalf("allowedTemplates missing %q", tpl)
		}
	}
	if allowedTemplates["not a template"] {
		t.Fatalf("unknown label unexpectedly accepted")
	}
}
