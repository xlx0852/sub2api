package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Fast mode（service_tier=priority/fast）下缓存写入与输入同倍率，因此 catalog 价格条目
// 必须能表达 cache_creation_input_token_cost_priority；缺字段会导致
// priceEntryToModelPricing 丢档、fast 请求按标准价少收。
func TestFallbackPricingCarriesCacheWritePriority(t *testing.T) {
	cases := map[string]float64{
		// 官方 Fast 表 cache-write 列（USD per MTok）
		"gpt-6-astra":   2.5e-05,
		"gpt-6-sol":     5e-06,
		"gpt-6-luna":    2.5e-07,
		"gpt-5.6-sol":   1e-05,
		"gpt-5.6-terra": 5e-06,
		"gpt-5.6-luna":  5e-07,
	}

	for model, want := range cases {
		entry, ok := ResolvePriceEntry(model)
		require.Truef(t, ok, "%s missing from fallback_pricing", model)
		require.NotZerof(t, entry.CacheCreationInputTokenCostPriority,
			"%s must declare a Fast-tier cache-write price", model)
		require.InDeltaf(t, want, entry.CacheCreationInputTokenCostPriority, 1e-15,
			"%s Fast-tier cache-write price", model)

		// 快速档倍率应与官方 “cache writes 与 input 同倍率” 一致。
		factor := entry.InputCostPerTokenPriority / entry.InputCostPerToken
		require.InDeltaf(t, entry.CacheCreationInputTokenCost*factor,
			entry.CacheCreationInputTokenCostPriority, 1e-15,
			"%s Fast-tier cache-write must follow the input factor", model)
	}
}

// 官方 cyber 表长上下文整列为 “-”，不得携带 above_272k / 长上下文字段，
// 否则会给官方未定义的档位按高价计费。
func TestCyberModelsHaveNoLongContextPricing(t *testing.T) {
	for _, model := range []string{"gpt-5.6-cyber", "gpt-5.5-cyber"} {
		entry, ok := ResolvePriceEntry(model)
		if !ok {
			continue // 未登记在 catalog fallback_pricing 时由定价镜像承担，跳过
		}
		require.Zerof(t, entry.LongContextInputTokenThreshold, "%s must not declare a long-context tier", model)
		require.Zerof(t, entry.LongContextInputCostMultiplier, "%s must not declare a long-context input multiplier", model)
		require.Zerof(t, entry.LongContextOutputCostMultiplier, "%s must not declare a long-context output multiplier", model)
	}
}
