package claude

import "github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"

func init() {
	DefaultModels = modelsFromCatalog()
	ModelIDOverrides = modelcatalog.AnthropicIDOverrides()
	ModelIDReverseOverrides = modelcatalog.AnthropicIDReverseOverrides()
	DefaultTestModel = modelcatalog.AnthropicDefaultTestModel()
}

func modelsFromCatalog() []Model {
	entries := modelcatalog.AnthropicModels()
	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		typ := e.Type
		if typ == "" {
			typ = "model"
		}
		out = append(out, Model{
			ID:          e.ID,
			Type:        typ,
			DisplayName: e.DisplayName,
			CreatedAt:   e.CreatedAt,
		})
	}
	return out
}
