package registry

import "testing"

func TestGetQoderModelsIncludesKnownTiers(t *testing.T) {
	models := GetQoderModels()
	byID := make(map[string]*ModelInfo, len(models))
	for _, model := range models {
		if model != nil {
			byID[model.ID] = model
		}
	}
	for _, id := range []string{"qoder/auto", "qoder/ultimate", "qoder/performance", "qoder/dmodel"} {
		if byID[id] == nil {
			t.Fatalf("missing model %q", id)
		}
	}
	if byID["qoder/ultimate"].Thinking == nil {
		t.Fatal("qoder/ultimate should advertise thinking support")
	}
}
