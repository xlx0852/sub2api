package billing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUsagePreservesGatewayIndependentBuckets(t *testing.T) {
	got := NormalizeUsage(Usage{
		InputTokens: 10, OutputTokens: 8, CacheReadTokens: 20, CacheCreationTokens: 30,
		CacheCreation5mTokens: 40, CacheCreation1hTokens: 40, ImageOutputTokens: 9,
	}, UsageNormalizationGateway)
	require.Equal(t, 10, got.InputTokens)
	require.Equal(t, 20, got.CacheReadTokens)
	require.Equal(t, 30, got.CacheCreationTokens)
	require.Equal(t, 30, got.CacheCreation5mTokens)
	require.Zero(t, got.CacheCreation1hTokens)
	require.Equal(t, 8, got.ImageOutputTokens)
	require.Equal(t, 10, ExclusiveInputTokens(got, UsageNormalizationGateway))
}

func TestNormalizeUsageMakesOpenAIInputBucketsExclusive(t *testing.T) {
	got := NormalizeUsage(Usage{
		InputTokens: 100, OutputTokens: 8, CacheReadTokens: 70, CacheCreationTokens: 50,
		ImageInputTokens: 120, ImageOutputTokens: 9,
	}, UsageNormalizationOpenAI)
	require.Equal(t, 70, got.CacheReadTokens)
	require.Equal(t, 30, got.CacheCreationTokens)
	require.Equal(t, 100, got.ImageInputTokens)
	require.Equal(t, 8, got.ImageOutputTokens)
	require.Zero(t, ExclusiveInputTokens(got, UsageNormalizationOpenAI))
}
