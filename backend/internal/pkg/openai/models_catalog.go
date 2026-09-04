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
			ID:                          e.ID,
			Object:                      obj,
			Created:                     e.Created,
			OwnedBy:                     owned,
			Type:                        typ,
			DisplayName:                 e.DisplayName,
			ContextWindow:               e.ContextWindow,
			MaxContextWindow:            e.MaxContextWindow,
			MaxOutputTokens:             e.MaxOutputTokens,
			SupportedReasoningLevels:    append([]string(nil), e.SupportedReasoningLevels...),
			DefaultReasoningLevel:       e.DefaultReasoningLevel,
			SupportsVerbosity:           e.SupportsVerbosity,
			DefaultVerbosity:            e.DefaultVerbosity,
			UseResponsesLite:            e.UseResponsesLite,
			ToolMode:                    e.ToolMode,
			MultiAgentVersion:           e.MultiAgentVersion,
			MultiAgentReasoningEffort:   e.MultiAgentReasoningEffort,
			SupportsParallelToolCalls:   e.SupportsParallelToolCalls,
			SupportsImageDetailOriginal: e.SupportsImageDetailOriginal,
			TruncationLimit:             e.TruncationLimit,
			PreferWebsockets:            e.PreferWebsockets,
			MinimalClientVersion:        e.MinimalClientVersion,
			SupportedInAPI:              e.SupportedInAPI,
		})
	}
	return out
}

func refreshDefaultModelsFromCatalog() {
	DefaultModels = modelsFromCatalog()
}

// CurrentDefaultModels resolves the active catalog snapshot.
func CurrentDefaultModels() []Model { return modelsFromCatalog() }

// CurrentDefaultTestModel resolves the active catalog snapshot.
func CurrentDefaultTestModel() string { return modelcatalog.OpenAIDefaultTestModel() }

func init() {
	refreshDefaultModelsFromCatalog()
}

// IsRetiredModelID reports whether the OpenAI model should be hidden from selectable model lists.
func IsRetiredModelID(id string) bool {
	return modelcatalog.IsOpenAIRetired(id)
}

// DefaultModelIDs returns the default model ID list
func DefaultModelIDs() []string {
	models := CurrentDefaultModels()
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel is the default model for testing OpenAI accounts (from catalog).
var DefaultTestModel = modelcatalog.OpenAIDefaultTestModel()
