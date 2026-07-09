package openai

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
)

func modelsFromCatalog() []Model {
	entries := modelcatalog.OpenAIModels()
	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		obj := e.Object
		if obj == "" {
			obj = "model"
		}
		typ := e.Type
		if typ == "" {
			typ = "model"
		}
		owned := e.OwnedBy
		if owned == "" {
			owned = "openai"
		}
		out = append(out, Model{
			ID:          e.ID,
			Object:      obj,
			Created:     e.Created,
			OwnedBy:     owned,
			Type:        typ,
			DisplayName: e.DisplayName,
		})
	}
	return out
}

func refreshDefaultModelsFromCatalog() {
	DefaultModels = modelsFromCatalog()
}

func init() {
	refreshDefaultModelsFromCatalog()
}

// IsRetiredModelID reports whether the OpenAI model should be hidden from selectable model lists.
func IsRetiredModelID(id string) bool {
	return modelcatalog.IsOpenAIRetired(id)
}

// DefaultModelIDs returns the default model ID list
func DefaultModelIDs() []string {
	models := DefaultModels
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel is the default model for testing OpenAI accounts (from catalog).
var DefaultTestModel = modelcatalog.OpenAIDefaultTestModel()
