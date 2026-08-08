package registry

// GetTraeModels returns the known Trae chat model catalogue.
func GetTraeModels() []*ModelInfo {
	const created = int64(1785542400) // 2026-08-01
	models := []struct {
		id      string
		display string
		context int
	}{
		{id: "Doubao-Seed-Code", display: "Doubao Seed Code", context: 128000},
		{id: "GLM-5.1", display: "GLM-5.1", context: 128000},
		{id: "MiniMax-M2.7", display: "MiniMax M2.7", context: 128000},
		{id: "Kimi-K2.6", display: "Kimi K2.6", context: 128000},
		{id: "DeepSeek-V4-Pro", display: "DeepSeek V4 Pro", context: 128000},
	}
	out := make([]*ModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, &ModelInfo{
			ID:                  model.id,
			Object:              "model",
			Created:             created,
			OwnedBy:             "trae",
			Type:                "trae",
			DisplayName:         model.display,
			Description:         model.display + " via Trae",
			ContextLength:       model.context,
			MaxCompletionTokens: 8192,
		})
	}
	return out
}
