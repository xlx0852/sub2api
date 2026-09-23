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

// 上游响应模型审计：记录上游声明的模型，并给出三态不一致结论。
func TestBuildUsageLogAuditsUpstreamResponseModel(t *testing.T) {
	base := func() RecordInput {
		return RecordInput{
			RequestID: "req-audit", Model: "gpt-6-astra", RequestedModel: "gpt-6-astra",
			User: &domain.User{ID: 1}, APIKey: &domain.APIKey{ID: 2}, Account: &domain.Account{ID: 3},
		}
	}

	t.Run("response declares a different model", func(t *testing.T) {
		input := base()
		input.UpstreamResponseModel = "gpt-5.6-sol"
		log := BuildUsageLog(input, nil, 1)
		require.NotNil(t, log.UpstreamResponseModel)
		require.Equal(t, "gpt-5.6-sol", *log.UpstreamResponseModel)
		require.NotNil(t, log.UpstreamModelMismatch)
		require.True(t, *log.UpstreamModelMismatch)
	})

	t.Run("response matches the requested model", func(t *testing.T) {
		input := base()
		input.UpstreamResponseModel = "gpt-6-astra"
		log := BuildUsageLog(input, nil, 1)
		require.NotNil(t, log.UpstreamModelMismatch)
		require.False(t, *log.UpstreamModelMismatch)
	})

	t.Run("comparison is case insensitive", func(t *testing.T) {
		input := base()
		input.UpstreamResponseModel = "GPT-6-Astra"
		log := BuildUsageLog(input, nil, 1)
		require.NotNil(t, log.UpstreamModelMismatch)
		require.False(t, *log.UpstreamModelMismatch)
	})

	t.Run("compared against the mapped model when mapping applied", func(t *testing.T) {
		input := base()
		input.UpstreamModel = "gpt-5.6-sol" // 账号映射后的送上游模型
		input.UpstreamResponseModel = "gpt-5.6-sol"
		log := BuildUsageLog(input, nil, 1)
		require.NotNil(t, log.UpstreamModelMismatch)
		require.False(t, *log.UpstreamModelMismatch)

		input.UpstreamResponseModel = "gpt-6-astra" // 与「送上游」不一致，而非与请求不一致
		log = BuildUsageLog(input, nil, 1)
		require.NotNil(t, log.UpstreamModelMismatch)
		require.True(t, *log.UpstreamModelMismatch)
	})

	t.Run("no declaration stays tri-state nil", func(t *testing.T) {
		log := BuildUsageLog(base(), nil, 1)
		require.Nil(t, log.UpstreamResponseModel)
		require.Nil(t, log.UpstreamModelMismatch)
	})
}
