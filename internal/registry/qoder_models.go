package registry

// GetQoderModels returns the static fallback catalogue used when Qoder's live
// model endpoint is unavailable.
func GetQoderModels() []*ModelInfo {
	const created = int64(1767196800)
	type modelDefinition struct {
		id          string
		display     string
		description string
		context     int
		thinking    *ThinkingSupport
	}
	fullThinking := &ThinkingSupport{
		Min: 1024, Max: 32000, ZeroAllowed: true, DynamicAllowed: true,
		Levels: []string{"low", "medium", "high", "max", "xhigh"},
	}
	highThinking := &ThinkingSupport{
		Min: 1024, Max: 32000, ZeroAllowed: true, DynamicAllowed: true,
		Levels: []string{"high", "max"},
	}
	definitions := []modelDefinition{
		{id: "auto", display: "Qoder Auto", description: "Automatically selects the best Qoder model", context: 180000},
		{id: "ultimate", display: "Qoder Ultimate", description: "Highest quality Qoder tier", context: 180000, thinking: fullThinking},
		{id: "performance", display: "Qoder Performance", description: "Balanced Qoder quality and speed tier", context: 272000, thinking: fullThinking},
		{id: "efficient", display: "Qoder Efficient", description: "Cost-efficient Qoder tier", context: 180000},
		{id: "lite", display: "Qoder Lite", description: "Fastest Qoder tier", context: 180000},
		{id: "qmodel", display: "Qwen3.7 Plus (Qoder)", description: "Qwen3.7 Plus via Qoder", context: 180000},
		{id: "qmodel_latest", display: "Qwen3.7 Max (Qoder)", description: "Qwen3.7 Max via Qoder", context: 180000},
		{id: "dmodel", display: "DeepSeek V4 Pro (Qoder)", description: "DeepSeek V4 Pro via Qoder", context: 180000, thinking: highThinking},
		{id: "dfmodel", display: "DeepSeek V4 Flash (Qoder)", description: "DeepSeek V4 Flash via Qoder", context: 180000, thinking: highThinking},
		{id: "gm51model", display: "GLM 5.1 (Qoder)", description: "GLM 5.1 via Qoder", context: 180000, thinking: highThinking},
		{id: "kmodel", display: "Kimi K2.6 (Qoder)", description: "Kimi K2.6 via Qoder", context: 256000},
		{id: "mmodel", display: "MiniMax M3 (Qoder)", description: "MiniMax M3 via Qoder", context: 180000},
	}
	models := make([]*ModelInfo, 0, len(definitions))
	for _, definition := range definitions {
		models = append(models, &ModelInfo{
			ID:                       "qoder/" + definition.id,
			Object:                   "model",
			Created:                  created,
			OwnedBy:                  "qoder",
			Type:                     "qoder",
			DisplayName:              definition.display,
			Description:              definition.description,
			ContextLength:            definition.context,
			SupportedInputModalities: []string{"TEXT", "IMAGE"},
			Thinking:                 definition.thinking,
		})
	}
	return models
}
