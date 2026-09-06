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
	for _, model := range models {
		id := strings.ToLower(model.ID)
		if !strings.HasPrefix(id, "gemini-") || strings.Contains(id, "image") || strings.Contains(id, "imagen") {
			t.Fatalf("unsupported Antigravity model published: %s", model.ID)
		}
	}
}
