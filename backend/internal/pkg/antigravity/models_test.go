package antigravity

import (
	"strings"
	"testing"
)

func TestDefaultModelsContainOnlyTextGemini(t *testing.T) {
	models := DefaultModels()
	if len(models) == 0 {
		t.Fatal("expected text Gemini Antigravity models")
	}
	modelMap := make(map[string]ClaudeModel, len(models))
	for _, model := range models {
		id := strings.ToLower(model.ID)
		if !strings.HasPrefix(id, "gemini-") || strings.Contains(id, "image") || strings.Contains(id, "imagen") {
			t.Fatalf("unsupported Antigravity model published: %s", model.ID)
		}
		modelMap[model.ID] = model
	}

	expectedModels := []string{
		"gemini-3.8-flash",
		"gemini-3.8-flash-high",
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.1-pro",
		"gemini-3.1-pro-high",
		"gemini-3.1-pro-low",
	}
	for _, exp := range expectedModels {
		if _, ok := modelMap[exp]; !ok {
			t.Errorf("expected model %q to be in Antigravity DefaultModels", exp)
		}
	}

	for _, unexp := range []string{"gemini-3-flash", "gemini-3.1-flash-lite"} {
		if _, ok := modelMap[unexp]; ok {
			t.Errorf("expected obsolete model %q to NOT be in Antigravity DefaultModels", unexp)
		}
	}
}

func TestIsGeminiReasoningModel(t *testing.T) {
	reasoningCases := map[string]bool{
		"gemini-3.8-flash":      false,
		"gemini-3.8-flash-high": true,
		"gemini-3.7-flash":      false,
		"gemini-3.7-flash-high": true,
		"gemini-3.6-flash":      false,
		"gemini-3.6-flash-high": true,
		"gemini-3.1-pro":        true,
		"gemini-3.1-pro-high":   true,
		"gemini-3.1-pro-low":    false,
	}

	for modelID, want := range reasoningCases {
		if got := IsGeminiReasoningModel(modelID); got != want {
			t.Errorf("IsGeminiReasoningModel(%q) = %v; want %v", modelID, got, want)
		}
	}
}
