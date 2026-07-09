package xai

import "github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"

// DefaultChatModel is the current flagship chat model used for aliases and empty-mapping fallbacks.
// Populated from modelcatalog; kept as a package var for call-site compatibility.
var DefaultChatModel = modelcatalog.GrokDefaultChatModel()

// Model describes an xAI model in OpenAI-compatible /models shape.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

func DefaultModels() []Model {
	entries := modelcatalog.GrokModels()
	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		obj := e.Object
		if obj == "" {
			obj = "model"
		}
		owned := e.OwnedBy
		if owned == "" {
			owned = "xai"
		}
		out = append(out, Model{
			ID:          e.ID,
			Object:      obj,
			Created:     e.Created,
			OwnedBy:     owned,
			DisplayName: e.DisplayName,
		})
	}
	return out
}

func DefaultModelIDs() []string {
	models := DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func DefaultModelMapping() map[string]string {
	return modelcatalog.GrokDefaultMapping()
}
