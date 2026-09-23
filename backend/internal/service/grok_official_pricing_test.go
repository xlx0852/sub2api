//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 官方 xAI 价目（docs.x.ai API Pricing）：长上下文阈值统一 200k，超过后整单
// 输入侧 ×2、输出 ×2（含 cached input）；Priority = 标准价 ×2。
func TestGrokOfficialTextPricing(t *testing.T) {
	svc := newTestBillingService()

	cases := []struct {
		model      string
		in, out    float64 // 短上下文单价（USD/token）
		cached     float64
		prioIn     float64
		prioOut    float64
		prioCached float64
	}{
		{"grok-4.7", 2e-06, 6e-06, 5e-07, 4e-06, 1.2e-05, 1e-06},
		{"grok-4.6", 2e-06, 6e-06, 5e-07, 4e-06, 1.2e-05, 1e-06},
		{"grok-4.5", 2e-06, 6e-06, 3e-07, 4e-06, 1.2e-05, 6e-07},
		{"grok-4.3", 1.25e-06, 2.5e-06, 2e-07, 2.5e-06, 5e-06, 4e-07},
		{"grok-build-0.1", 1e-06, 2e-06, 2e-07, 2e-06, 4e-06, 4e-07},
		{"grok-4.20-0309-reasoning", 1.25e-06, 2.5e-06, 2e-07, 2.5e-06, 5e-06, 4e-07},
		{"grok-4.20-0309-non-reasoning", 1.25e-06, 2.5e-06, 2e-07, 2.5e-06, 5e-06, 4e-07},
		{"grok-4.20-multi-agent-0309", 1.25e-06, 2.5e-06, 2e-07, 2.5e-06, 5e-06, 4e-07},
	}

	// 各 10k，合计 30k < 200k，隔离长上下文，只验证短上下文与 priority。
	tokens := UsageTokens{InputTokens: 10_000, OutputTokens: 10_000, CacheReadTokens: 10_000}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			std, err := svc.CalculateCostWithServiceTier(tc.model, tokens, 1.0, "")
			require.NoErrorf(t, err, "%s must resolve a fallback price", tc.model)
			require.InDeltaf(t, 10_000*tc.in, std.InputCost, 1e-12, "%s input", tc.model)
			require.InDeltaf(t, 10_000*tc.out, std.OutputCost, 1e-12, "%s output", tc.model)
			require.InDeltaf(t, 10_000*tc.cached, std.CacheReadCost, 1e-12, "%s cached input", tc.model)

			prio, err := svc.CalculateCostWithServiceTier(tc.model, tokens, 1.0, "priority")
			require.NoError(t, err)
			require.InDeltaf(t, 10_000*tc.prioIn, prio.InputCost, 1e-12, "%s priority input", tc.model)
			require.InDeltaf(t, 10_000*tc.prioOut, prio.OutputCost, 1e-12, "%s priority output", tc.model)
			require.InDeltaf(t, 10_000*tc.prioCached, prio.CacheReadCost, 1e-12, "%s priority cached input", tc.model)

			// 官方：Priority 就是标准价 ×2。
			require.InDeltaf(t, std.TotalCost*2, prio.TotalCost, 1e-9, "%s priority must be 2x standard", tc.model)
		})
	}
}

// 长上下文：prompt 达到 200k 后整单按长价（输入侧 ×2、输出 ×2）。
func TestGrokOfficialLongContextTier(t *testing.T) {
	svc := newTestBillingService()

	cases := []struct {
		model   string
		in, out float64
		cached  float64
	}{
		{"grok-4.7", 2e-06, 6e-06, 5e-07},
		{"grok-4.5", 2e-06, 6e-06, 3e-07},
		{"grok-4.3", 1.25e-06, 2.5e-06, 2e-07},
		{"grok-build-0.1", 1e-06, 2e-06, 2e-07},
		{"grok-4.20-0309-reasoning", 1.25e-06, 2.5e-06, 2e-07},
	}

	// 150k 输入 + 60k 命中缓存 = 210k > 200k，触发整单长价。
	long := UsageTokens{InputTokens: 150_000, OutputTokens: 5_000, CacheReadTokens: 60_000}
	// 短上下文对照：同为 150k 输入但总输入 150k < 200k。
	short := UsageTokens{InputTokens: 150_000, OutputTokens: 5_000}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			longCost, err := svc.CalculateCostWithServiceTier(tc.model, long, 1.0, "")
			require.NoError(t, err)
			require.InDeltaf(t, 150_000*tc.in*2, longCost.InputCost, 1e-9, "%s long input must be 2x", tc.model)
			require.InDeltaf(t, 5_000*tc.out*2, longCost.OutputCost, 1e-9, "%s long output must be 2x", tc.model)
			require.InDeltaf(t, 60_000*tc.cached*2, longCost.CacheReadCost, 1e-9, "%s long cached input must be 2x", tc.model)

			shortCost, err := svc.CalculateCostWithServiceTier(tc.model, short, 1.0, "")
			require.NoError(t, err)
			require.InDeltaf(t, 150_000*tc.in, shortCost.InputCost, 1e-9, "%s below threshold stays short", tc.model)
		})
	}
}

// 官方定价不随长上下文改变倍率：阈值以下为 1x，以上才是 2x。
func TestGrokLongContextThresholdIsExactly200k(t *testing.T) {
	svc := newTestBillingService()

	below := UsageTokens{InputTokens: 199_999}
	above := UsageTokens{InputTokens: 200_001}

	belowCost, err := svc.CalculateCostWithServiceTier("grok-4.3", below, 1.0, "")
	require.NoError(t, err)
	require.InDelta(t, 199_999*1.25e-06, belowCost.InputCost, 1e-9)

	aboveCost, err := svc.CalculateCostWithServiceTier("grok-4.3", above, 1.0, "")
	require.NoError(t, err)
	require.InDelta(t, 200_001*1.25e-06*2, aboveCost.InputCost, 1e-9)
}

// Imagine 图片：官方 image-2.0 为 1K low 0.04 / med 0.06、2K low 0.06 / med 0.08。
// 本仓库尚无质量维度，取默认（auto/medium）档。
func TestGrokImagineImage20Pricing(t *testing.T) {
	svc := newTestBillingService()

	oneK := svc.CalculateImageCost("grok-imagine-image-2.0", "1K", 1, nil, 1.0)
	require.InDelta(t, 0.06, oneK.TotalCost, 1e-9)

	twoK := svc.CalculateImageCost("grok-imagine-image-2.0", "2K", 1, nil, 1.0)
	require.InDelta(t, 0.08, twoK.TotalCost, 1e-9)

	// 既有 Imagine 价保持不变
	require.InDelta(t, 0.02, svc.CalculateImageCost("grok-imagine-image", "1K", 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.02, svc.CalculateImageCost("grok-imagine-image", "2K", 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.05, svc.CalculateImageCost("grok-imagine-image-quality", "1K", 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.07, svc.CalculateImageCost("grok-imagine-image-quality", "2K", 1, nil, 1.0).TotalCost, 1e-9)

	// 视频价与官方一致（每秒）
	require.InDelta(t, 0.05, svc.CalculateVideoCost("grok-imagine-video", "480p", 1, 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.07, svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.08, svc.CalculateVideoCost("grok-imagine-video-1.5", "480p", 1, 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.14, svc.CalculateVideoCost("grok-imagine-video-1.5", "720p", 1, 1, nil, 1.0).TotalCost, 1e-9)
	require.InDelta(t, 0.25, svc.CalculateVideoCost("grok-imagine-video-1.5", "1080p", 1, 1, nil, 1.0).TotalCost, 1e-9)
}
