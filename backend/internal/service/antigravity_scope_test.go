package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityQuotaWindowDuration(t *testing.T) {
	tests := []struct {
		window string
		label  string
		want   time.Duration
		key    string
	}{
		{"5h", "", 5 * time.Hour, "5h"},
		{"", "Daily limit", 24 * time.Hour, "1d"},
		{"weekly", "", 7 * 24 * time.Hour, "7d"},
		{"unknown", "", 0, ""},
	}
	for _, test := range tests {
		got, key := antigravityQuotaWindowDuration(test.window, test.label)
		require.Equal(t, test.want, got)
		require.Equal(t, test.key, key)
	}
}

func TestAntigravityModelScope(t *testing.T) {
	svc := &AntigravityGatewayService{}
	require.True(t, svc.IsModelSupported("gemini-3.8-flash"))
	require.True(t, svc.IsModelSupported("gemini-3.8-flash-high"))
	require.True(t, svc.IsModelSupported("gemini-3.7-flash"))
	require.True(t, svc.IsModelSupported("gemini-3.7-flash-high"))
	require.True(t, svc.IsModelSupported("gemini-3.6-flash"))
	require.True(t, svc.IsModelSupported("gemini-3.6-flash-high"))
	require.True(t, svc.IsModelSupported("gemini-3.1-pro"))
	require.True(t, svc.IsModelSupported("gemini-3.1-pro-high"))
	require.True(t, svc.IsModelSupported("gemini-3.1-pro-low"))
	require.True(t, svc.IsModelSupported("gemini-3.1-flash-lite"))
	require.True(t, svc.IsModelSupported("gemini-3-flash"))
	require.False(t, svc.IsModelSupported("claude-sonnet-4-6"))
	require.False(t, svc.IsModelSupported("claude-opus-4-6-thinking"))
	require.False(t, svc.IsModelSupported("gemini-3.1-flash-image"))
	require.False(t, svc.IsModelSupported("imagen-4"))
}

func TestNormalizeAntigravityUltraLiteTier(t *testing.T) {
	require.Equal(t, "ULTRA_LITE", normalizeTier("g1-ultra-lite-tier"))
}

func TestNormalizeAntigravityTierIncludesUltraLite(t *testing.T) {
	require.Equal(t, "ULTRA_LITE", normalizeTier("g1-ultra-lite-tier"))
}

func TestAntigravityQuotaFiltersUnsupportedModels(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	remaining25 := 0.25
	remaining50 := 0.5
	info := fetcher.buildQuotaSummaryUsageInfo(&antigravity.RetrieveUserQuotaSummaryResponse{
		Groups: []antigravity.QuotaSummaryGroup{
			{DisplayName: "Gemini Models", Buckets: []antigravity.QuotaSummaryBucket{{BucketID: "five-hour", DisplayName: "5 Hour Limit", Window: "5h", RemainingFraction: &remaining25}}},
			{DisplayName: "Claude and GPT Models", Buckets: []antigravity.QuotaSummaryBucket{{BucketID: "weekly", RemainingFraction: &remaining50}}},
			{DisplayName: "Gemini Image Models", Buckets: []antigravity.QuotaSummaryBucket{{BucketID: "image", RemainingFraction: &remaining50}}},
		},
	}, "free-tier", "FREE", nil)

	require.Len(t, info.AntigravityQuotaGroups, 1)
	require.Equal(t, "Gemini Models", info.AntigravityQuotaGroups[0].Label)
	require.Equal(t, 0.25, info.AntigravityQuotaGroups[0].Buckets[0].RemainingFraction)
	require.NotNil(t, info.FiveHour)
	require.Equal(t, float64(75), info.FiveHour.Utilization)
	require.Equal(t, 75, info.AntigravityQuota["gemini-3.1-pro-high"].Utilization)
	require.Equal(t, 75, info.AntigravityQuota["gemini-3.8-flash"].Utilization)
}

func TestAntigravityQuotaClampsRemainingFraction(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}
	low, high := -0.1, 1.1
	info := fetcher.buildQuotaSummaryUsageInfo(&antigravity.RetrieveUserQuotaSummaryResponse{Groups: []antigravity.QuotaSummaryGroup{{
		DisplayName: "Gemini Models",
		Buckets:     []antigravity.QuotaSummaryBucket{{BucketID: "low", RemainingFraction: &low}, {BucketID: "high", RemainingFraction: &high}},
	}}}, "", "", nil)
	require.Equal(t, float64(0), info.AntigravityQuotaGroups[0].Buckets[0].RemainingFraction)
	require.Equal(t, float64(1), info.AntigravityQuotaGroups[0].Buckets[1].RemainingFraction)
}
