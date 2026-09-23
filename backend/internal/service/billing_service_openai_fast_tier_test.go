//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 官方 Fast 档表对 cache writes 与 input 同倍率：
// gpt-6-sol 标准 2 / 0.2 / 2.5 / 10（输入/缓存读/缓存写/输出）→ Fast 4 / 0.4 / 5 / 20。
// 回归点：catalog fallback_pricing 走 priceEntryToModelPricing 时不得丢掉缓存写快速档，
// 否则 fast 请求的 cache-write 会按标准价计费（少收一半）。
func TestCalculateCostWithServiceTier_FastModeScalesCacheWrite(t *testing.T) {
	svc := newTestBillingService()

	// 各 50K，合计 150K < 272K，避免触发长上下文整单倍率，隔离 fast 档本身的行为。
	tokens := UsageTokens{
		InputTokens:         50_000,
		OutputTokens:        50_000,
		CacheCreationTokens: 50_000,
		CacheReadTokens:     50_000,
	}

	std, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "")
	require.NoError(t, err)
	fast, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "priority")
	require.NoError(t, err)

	// 标准档 = 官方 2 / 0.2 / 2.5 / 10 USD per MTok
	require.InDelta(t, 50_000*2e-06, std.InputCost, 1e-12, "standard input")
	require.InDelta(t, 50_000*2e-07, std.CacheReadCost, 1e-12, "standard cache read")
	require.InDelta(t, 50_000*2.5e-06, std.CacheCreationCost, 1e-12, "standard cache write")
	require.InDelta(t, 50_000*1e-05, std.OutputCost, 1e-12, "standard output")

	// Fast 档 = 官方 4 / 0.4 / 5 / 20 USD per MTok
	require.InDelta(t, 50_000*4e-06, fast.InputCost, 1e-12, "fast input")
	require.InDelta(t, 50_000*4e-07, fast.CacheReadCost, 1e-12, "fast cache read")
	require.InDelta(t, 50_000*5e-06, fast.CacheCreationCost, 1e-12, "fast cache write")
	require.InDelta(t, 50_000*2e-05, fast.OutputCost, 1e-12, "fast output")
}

// 官方 Fast 档表不含 cyber 模型（长上下文一列也是 “-”），因此请求 service_tier=priority
// 时不得套用任何兜底放大倍率，否则会多收。
func TestCalculateCostWithServiceTier_ModelWithoutFastTierIsNotUpscaled(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:         50_000,
		OutputTokens:        50_000,
		CacheCreationTokens: 50_000,
		CacheReadTokens:     50_000,
	}

	std, err := svc.CalculateCostWithServiceTier("gpt-5.6-cyber", tokens, 1.0, "")
	require.NoError(t, err)
	fast, err := svc.CalculateCostWithServiceTier("gpt-5.6-cyber", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, std.TotalCost, fast.TotalCost, 1e-12,
		"model without a declared Fast tier must be billed at standard rates, not a blanket multiplier")
}

// Flex 仍是官方文档中的全模型 0.5x，不受上面的收紧影响。
func TestCalculateCostWithServiceTier_FlexStillHalves(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:         50_000,
		OutputTokens:        50_000,
		CacheCreationTokens: 50_000,
		CacheReadTokens:     50_000,
	}

	std, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "")
	require.NoError(t, err)
	flex, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "flex")
	require.NoError(t, err)

	require.InDelta(t, std.TotalCost*0.5, flex.TotalCost, 1e-12, "flex must stay 0.5x")
}

// 长上下文（>272K）与 Fast 叠乘，与官方 $16 / $60（gpt-5.6 Sol）口径一致。
func TestCalculateCostWithServiceTier_FastAndLongContextCompose(t *testing.T) {
	svc := newTestBillingService()

	// 1M 输入使整单落入长上下文档。
	tokens := UsageTokens{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}

	_, err := svc.CalculateCostWithServiceTier("gpt-5.6-sol", tokens, 1.0, "")
	require.NoError(t, err)
	fast, err := svc.CalculateCostWithServiceTier("gpt-5.6-sol", tokens, 1.0, "priority")
	require.NoError(t, err)

	// 官方 Fast 长上下文 = $16 in / $60 out per MTok
	require.InDelta(t, 16.0, fast.InputCost, 1e-9, "fast long-context input")
	require.InDelta(t, 60.0, fast.OutputCost, 1e-9, "fast long-context output")
}
