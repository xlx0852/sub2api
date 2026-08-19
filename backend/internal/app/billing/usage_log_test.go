package billing

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildUsageLogPreservesOpenAIOptionalFields(t *testing.T) {
	groupID := int64(3)
	input := RecordInput{
		RequestID: "req-1", Model: "gpt-5", RequestedModel: "gpt-5",
		Usage: Usage{InputTokens: 10, OutputTokens: 5},
		User:  &domain.User{ID: 1}, APIKey: &domain.APIKey{ID: 2, GroupID: &groupID}, Account: &domain.Account{ID: 4},
		BillingType: 1, Compact: true, CyberBlocked: true, OpenAIWSMode: true,
		VideoUsage: true, VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 5,
	}
	log := BuildUsageLog(input, &CostBreakdown{ActualCost: 0.25, BillingMode: "video"}, 1.5)
	require.Equal(t, domain.RequestTypeCyberBlocked, log.RequestType)
	require.True(t, log.OpenAIWSMode)
	require.Equal(t, 1, log.VideoCount)
	require.Equal(t, "720p", *log.VideoResolution)
	require.Equal(t, 5, *log.VideoDurationSeconds)
	require.Equal(t, "video", *log.BillingMode)
	require.Equal(t, 0.25, log.ActualCost)
}

func TestBuildUsageLogPreservesVideoAttributionWhenCountIsZero(t *testing.T) {
	input := RecordInput{
		RequestID: "req-video-zero", Model: "grok-imagine-video", RequestedModel: "grok-imagine-video",
		User: &domain.User{ID: 1}, APIKey: &domain.APIKey{ID: 2}, Account: &domain.Account{ID: 3},
		VideoUsage: true, VideoResolution: "480p", VideoDurationSeconds: 1,
	}
	log := BuildUsageLog(input, &CostBreakdown{BillingMode: "video"}, 1)
	require.Zero(t, log.VideoCount)
	require.Equal(t, "480p", *log.VideoResolution)
	require.Equal(t, 1, *log.VideoDurationSeconds)
	require.Nil(t, log.ImageSize)
}

func TestBuildUsageLogSnapshotsPricingCurrencyAndDefaultsLegacyCostToUSD(t *testing.T) {
	input := RecordInput{User: &domain.User{ID: 1}, APIKey: &domain.APIKey{ID: 2}, Account: &domain.Account{ID: 3}}

	cny := BuildUsageLog(input, &CostBreakdown{PricingCurrency: domain.PricingCurrencyCNY}, 1)
	require.NotNil(t, cny.PricingCurrency)
	require.Equal(t, "CNY", *cny.PricingCurrency)

	legacy := BuildUsageLog(input, &CostBreakdown{}, 1)
	require.NotNil(t, legacy.PricingCurrency)
	require.Equal(t, "USD", *legacy.PricingCurrency)
}
