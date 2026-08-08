package registry

import "testing"

func TestGetTraeModelsIncludesCLICloudModels(t *testing.T) {
	models := GetTraeModels()
	if len(models) == 0 {
		t.Fatal("GetTraeModels returned no models")
	}
	want := map[string]bool{
		"Doubao-Seed-Code": false,
		"GLM-5.1":          false,
		"MiniMax-M2.7":     false,
		"Kimi-K2.6":        false,
		"DeepSeek-V4-Pro":  false,
	}
	for _, model := range models {
		if model != nil {
			if _, ok := want[model.ID]; ok {
				want[model.ID] = true
			}
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("GetTraeModels does not include %s", id)
		}
	}
}

func TestGetStaticModelDefinitionsByChannelIncludesTrae(t *testing.T) {
	models := GetStaticModelDefinitionsByChannel("trae")
	if len(models) != len(GetTraeModels()) {
		t.Fatalf("Trae static model count = %d, want %d", len(models), len(GetTraeModels()))
	}
}
