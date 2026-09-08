package domain

import (
	"strings"
	"testing"
)

func TestDefaultAntigravityModelMapping_ModernGeminiModels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"gemini-3.8-flash":      "gemini-3.8-flash-high",
		"gemini-3.8-flash-high": "gemini-3.8-flash-high",
		"gemini-3.7-flash":      "gemini-3.7-flash-high",
		"gemini-3.7-flash-high": "gemini-3.7-flash-high",
		"gemini-3.6-flash":      "gemini-3.6-flash-high",
		"gemini-3.6-flash-high": "gemini-3.6-flash-high",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultAntigravityModelMapping_ExcludesUnsupportedAndClaudeModels(t *testing.T) {
	t.Parallel()

	unsupported := []string{
		"gemini-3-flash",
		"gemini-3-flash-preview",
		"gemini-3.1-flash-lite",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
		"gemini-3-pro-high",
		"gemini-3-pro-low",
	}
	for _, m := range unsupported {
		if _, exists := DefaultAntigravityModelMapping[m]; exists {
			t.Fatalf("expected unsupported model %q to be excluded from DefaultAntigravityModelMapping", m)
		}
	}

	for k, v := range DefaultAntigravityModelMapping {
		if strings.Contains(k, "claude") || strings.Contains(v, "claude") {
			t.Fatalf("unexpected Claude model in Antigravity mapping: %q -> %q", k, v)
		}
		if strings.Contains(k, "image") || strings.Contains(v, "image") {
			t.Fatalf("unexpected image model in Antigravity mapping: %q -> %q", k, v)
		}
	}
}

func TestDefaultAntigravityModelMapping_Gemini31ProAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		AntigravityGemini31ProAgentModel: AntigravityGemini31ProAgentModel,
		"gemini-3.1-pro":                 AntigravityGemini31ProAgentModel,
		"gemini-3.1-pro-high":            AntigravityGemini31ProAgentModel,
		"gemini-3.1-pro-preview":         AntigravityGemini31ProAgentModel,
		"gemini-3.1-pro-low":             "gemini-3.1-pro-low",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}
