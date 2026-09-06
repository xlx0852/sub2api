package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityModelScope(t *testing.T) {
	svc := &AntigravityGatewayService{}
	require.True(t, svc.IsModelSupported("gemini-3-flash"))
	require.True(t, svc.IsModelSupported("gemini-3.1-pro-high"))
	require.False(t, svc.IsModelSupported("claude-sonnet-4-6"))
	require.False(t, svc.IsModelSupported("gemini-3.1-flash-image"))
	require.False(t, svc.IsModelSupported("imagen-4"))
}

func TestAntigravityQuotaFiltersUnsupportedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	info := fetcher.buildUsageInfo(&antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			"gemini-3-flash":         {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 0.25}},
			"claude-sonnet-4-6":      {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 0.5}},
			"gemini-3.1-flash-image": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 0.5}},
		},
		DeprecatedModelIDs: map[string]antigravity.DeprecatedModelInfo{
			"gemini-2.5-flash":  {NewModelID: "gemini-3-flash"},
			"claude-sonnet-4-5": {NewModelID: "claude-sonnet-4-6"},
		},
	}, "free-tier", "FREE", nil)

	require.Equal(t, 75, info.AntigravityQuota["gemini-3-flash"].Utilization)
	require.NotContains(t, info.AntigravityQuota, "claude-sonnet-4-6")
	require.NotContains(t, info.AntigravityQuota, "gemini-3.1-flash-image")
	require.Equal(t, map[string]string{"gemini-2.5-flash": "gemini-3-flash"}, info.ModelForwardingRules)
}

func TestAntigravityQuotaClampsRemainingFraction(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	info := fetcher.buildUsageInfo(&antigravity.FetchAvailableModelsResponse{Models: map[string]antigravity.ModelInfo{
		"gemini-low":  {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: -0.1}},
		"gemini-high": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 1.1}},
	}}, "", "", nil)
	require.Equal(t, 100, info.AntigravityQuota["gemini-low"].Utilization)
	require.Equal(t, 0, info.AntigravityQuota["gemini-high"].Utilization)
}
