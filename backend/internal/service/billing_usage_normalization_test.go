//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIUsageForBilling(t *testing.T) {
	usage := OpenAIUsage{
		InputTokens:              100,
		ImageInputTokens:         150,
		OutputTokens:             20,
		CacheReadInputTokens:     80,
		CacheCreationInputTokens: 50,
		ImageOutputTokens:        30,
	}

	normalizeOpenAIUsageForBilling(&usage)

	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 100, usage.ImageInputTokens)
	require.Equal(t, 20, usage.OutputTokens)
	require.Equal(t, 80, usage.CacheReadInputTokens)
	require.Equal(t, 20, usage.CacheCreationInputTokens)
	require.Equal(t, 20, usage.ImageOutputTokens)
}

func TestNormalizeUsageForBillingClearsNegativeTokens(t *testing.T) {
	openAIUsage := OpenAIUsage{
		InputTokens:              -1,
		ImageInputTokens:         -2,
		OutputTokens:             -3,
		CacheCreationInputTokens: -4,
		CacheReadInputTokens:     -5,
		ImageOutputTokens:        -6,
	}
	normalizeOpenAIUsageForBilling(&openAIUsage)
	require.Equal(t, OpenAIUsage{}, openAIUsage)

	claudeUsage := ClaudeUsage{
		InputTokens:              -1,
		OutputTokens:             -2,
		CacheCreationInputTokens: -3,
		CacheReadInputTokens:     -4,
		CacheCreation5mTokens:    -5,
		CacheCreation1hTokens:    -6,
		ImageOutputTokens:        -7,
	}
	normalizeClaudeUsageForBilling(&claudeUsage)
	require.Equal(t, ClaudeUsage{}, claudeUsage)
}

func TestNormalizeClaudeUsageForBillingCapsCacheBreakdown(t *testing.T) {
	usage := ClaudeUsage{
		OutputTokens:             10,
		CacheCreationInputTokens: 100,
		CacheCreation5mTokens:    80,
		CacheCreation1hTokens:    50,
		ImageOutputTokens:        20,
	}

	normalizeClaudeUsageForBilling(&usage)

	require.Equal(t, 80, usage.CacheCreation5mTokens)
	require.Equal(t, 20, usage.CacheCreation1hTokens)
	require.Equal(t, 10, usage.ImageOutputTokens)
}
