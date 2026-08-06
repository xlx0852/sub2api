package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
)

// Gemini upstream constants kept for residual AI Studio / legacy test paths.
// Standalone Gemini supplier and Gemini CLI OAuth are retired.
const (
	GeminiAIStudioBaseURL = "https://generativelanguage.googleapis.com"
	GeminiCliBaseURL      = "https://cloudcode-pa.googleapis.com"
	GeminiCLIUserAgent    = "GeminiCLI/0.1.5 (Windows; AMD64)"

	GeminiTierGoogleOneFree    = "google_one_free"
	GeminiTierGoogleAIPro      = "google_ai_pro"
	GeminiTierGoogleAIUltra    = "google_ai_ultra"
	GeminiTierGCPStandard      = "gcp_standard"
	GeminiTierGCPEnterprise    = "gcp_enterprise"
	GeminiTierAIStudioFree     = "aistudio_free"
	GeminiTierAIStudioPaid     = "aistudio_paid"
	GeminiTierGoogleOneUnknown = "google_one_unknown"
)

// GeminiModel is a minimal model descriptor for admin test UI fallbacks.
type GeminiModel struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

func GeminiDefaultModels() []GeminiModel {
	entries := modelcatalog.GeminiModels()
	out := make([]GeminiModel, 0, len(entries))
	for _, e := range entries {
		id := e.ID
		if id == "" || strings.Contains(id, "customtools") {
			continue
		}
		typ := e.Type
		if typ == "" {
			typ = "model"
		}
		out = append(out, GeminiModel{ID: id, Type: typ, DisplayName: e.DisplayName, CreatedAt: e.CreatedAt})
	}
	return out
}

func GeminiDefaultTestModel() string { return modelcatalog.GeminiDefaultTestModel() }

func canonicalGeminiTierID(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch s {
	case GeminiTierGoogleOneFree, GeminiTierGoogleAIPro, GeminiTierGoogleAIUltra,
		GeminiTierGCPStandard, GeminiTierGCPEnterprise, GeminiTierAIStudioFree,
		GeminiTierAIStudioPaid, GeminiTierGoogleOneUnknown:
		return s
	default:
		return strings.TrimSpace(raw)
	}
}

func canonicalGeminiTierIDForOAuthType(oauthType, tierID string) string {
	return canonicalGeminiTierID(tierID)
}

