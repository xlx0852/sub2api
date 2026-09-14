package antigravity

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
)

type modelDef struct {
	ID          string
	DisplayName string
	CreatedAt   string
	IsReasoning bool
}

var geminiModels []modelDef

func init() {
	for _, entry := range modelcatalog.AntigravityModels() {
		if !strings.EqualFold(strings.TrimSpace(entry.Family), "gemini") && !strings.HasPrefix(entry.ID, "gemini-") {
			continue
		}
		modelLower := strings.ToLower(strings.TrimSpace(entry.ID))
		if strings.Contains(modelLower, "image") || strings.Contains(modelLower, "imagen") {
			continue
		}
		geminiModels = append(geminiModels, modelDef{
			ID: entry.ID, DisplayName: entry.DisplayName, CreatedAt: entry.CreatedAt, IsReasoning: entry.IsReasoning,
		})
	}
}

// ClaudeModel is retained as the response shape used by the existing /v1/models
// catalog endpoint. The catalog now contains text Gemini Antigravity models only.
type ClaudeModel struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

func isPublicAntigravityModelID(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || strings.HasPrefix(id, "MODEL_") || id == "gemini-pro-agent" {
		return false
	}
	for _, suffix := range []string{"-high", "-low", "-minimal"} {
		if strings.HasSuffix(id, suffix) {
			return false
		}
	}
	return true
}

func DefaultModels() []ClaudeModel {
	result := make([]ClaudeModel, 0, len(geminiModels))
	for _, model := range geminiModels {
		if !isPublicAntigravityModelID(model.ID) {
			continue
		}
		result = append(result, ClaudeModel{ID: model.ID, Type: "model", DisplayName: model.DisplayName, CreatedAt: model.CreatedAt})
	}
	return result
}

type GeminiModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
}

type GeminiModelsListResponse struct {
	Models []GeminiModel `json:"models"`
}

var defaultGeminiMethods = []string{"generateContent", "streamGenerateContent"}

func DefaultGeminiModels() []GeminiModel {
	result := make([]GeminiModel, 0, len(geminiModels))
	for _, model := range geminiModels {
		if !isPublicAntigravityModelID(model.ID) {
			continue
		}
		result = append(result, GeminiModel{Name: "models/" + model.ID, DisplayName: model.DisplayName, SupportedGenerationMethods: defaultGeminiMethods})
	}
	return result
}

func FallbackGeminiModelsList() GeminiModelsListResponse {
	return GeminiModelsListResponse{Models: DefaultGeminiModels()}
}

func FallbackGeminiModel(model string) GeminiModel {
	if model == "" {
		return GeminiModel{Name: "models/unknown", SupportedGenerationMethods: defaultGeminiMethods}
	}
	name := model
	if !strings.HasPrefix(model, "models/") {
		name = "models/" + model
	}
	return GeminiModel{Name: name, SupportedGenerationMethods: defaultGeminiMethods}
}

func IsGeminiReasoningModel(modelID string) bool {
	lowerID := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "models/")
	for _, model := range geminiModels {
		if strings.EqualFold(lowerID, model.ID) {
			return model.IsReasoning
		}
	}
	matched := false
	matchedLen := 0
	for _, model := range geminiModels {
		if strings.Contains(lowerID, strings.ToLower(model.ID)) && len(model.ID) > matchedLen {
			matched = model.IsReasoning
			matchedLen = len(model.ID)
		}
	}
	return matched
}
