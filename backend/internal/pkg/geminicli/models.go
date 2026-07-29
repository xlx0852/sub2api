package geminicli

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
)

// Model represents a selectable Gemini model for UI/testing purposes.
// Keep JSON fields consistent with existing frontend expectations.
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels is the curated Gemini model list used by the admin UI "test account" flow.
// Sourced from modelcatalog (short IDs without models/ prefix).
var DefaultModels []Model

// DefaultTestModel is the default model to preselect in test flows.
var DefaultTestModel = modelcatalog.GeminiDefaultTestModel()

func init() {
	DefaultModels = modelsFromCatalog()
	DefaultTestModel = modelcatalog.GeminiDefaultTestModel()
}

func CurrentDefaultModels() []Model { return modelsFromCatalog() }

func CurrentDefaultTestModel() string { return modelcatalog.GeminiDefaultTestModel() }

func modelsFromCatalog() []Model {
	entries := modelcatalog.GeminiModels()
	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		// Skip gemini native-only customtools variants that are not in CLI short-id list when id empty
		id := e.ID
		if id == "" || strings.Contains(id, "customtools") {
			continue
		}
		typ := e.Type
		if typ == "" {
			typ = "model"
		}
		out = append(out, Model{
			ID:          id,
			Type:        typ,
			DisplayName: e.DisplayName,
			CreatedAt:   e.CreatedAt,
		})
	}
	return out
}
