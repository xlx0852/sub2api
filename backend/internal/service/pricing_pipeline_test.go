//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestNormalizePricingPipelineMode(t *testing.T) {
	cases := map[string]string{
		"":            PricingPipelineLegacy,
		"  ":          PricingPipelineLegacy,
		"legacy":      PricingPipelineLegacy,
		"SHADOW":      PricingPipelineShadow,
		" unified ":   PricingPipelineUnified,
		"unknown-xyz": PricingPipelineLegacy,
	}
	for in, want := range cases {
		require.Equal(t, want, NormalizePricingPipelineMode(in), "input %q", in)
	}
}

func TestNormalizePricingFxMode(t *testing.T) {
	require.Equal(t, FxModeAuto, NormalizePricingFxMode("fx_auto"))
	require.Equal(t, FxModeCnyNative, NormalizePricingFxMode("CNY_NATIVE"))
	require.Equal(t, FxModeManual, NormalizePricingFxMode(""))
	require.Equal(t, FxModeManual, NormalizePricingFxMode("whatever"), "未知值必须降级为不换汇")
}

// hinoki-offensive 的真实数字：$0.80 / $0.08 / $1.20，汇率 7.0。
func TestDeriveSellPriceHinokiAtRate7(t *testing.T) {
	cost := PriceCost{
		Vendor:            "hinoki",
		ModelPattern:      "hinoki-offensive",
		Currency:          domain.PricingCurrencyUSD,
		InputPerMTok:      0.80,
		CachedPerMTok:     0.08,
		OutputPerMTok:     1.20,
		CacheWritePerMTok: 0.80,
	}
	derived, err := DeriveSellPrice(cost, 0, 7.0)
	require.NoError(t, err)
	require.InDelta(t, 5.6e-6, derived.InputPerToken, 1e-15)
	require.InDelta(t, 0.56e-6, derived.CachedPerToken, 1e-15)
	require.InDelta(t, 8.4e-6, derived.OutputPerToken, 1e-15)
	require.InDelta(t, 7.0, derived.EffectiveRate, 1e-12)
}

func TestDeriveSellPriceAppliesDiscountAndMarkup(t *testing.T) {
	cost := PriceCost{
		Vendor:        "refuseless",
		ModelPattern:  "deepseek-v4.1-flash-abliterated",
		Currency:      domain.PricingCurrencyUSD,
		InputPerMTok:  0.14,
		CachedPerMTok: 0.035,
		OutputPerMTok: 0.50,
		DiscountPct:   29,
	}
	derived, err := DeriveSellPrice(cost, 10, 7.2)
	require.NoError(t, err)
	// 0.14 × 0.71 × 7.2 × 1.1 = 0.787248
	require.InDelta(t, 0.14*0.71*7.2*1.1, derived.InputPerToken*1e6, 1e-9)
	require.InDelta(t, 0.50*0.71*7.2*1.1, derived.OutputPerToken*1e6, 1e-9)
}

func TestDeriveSellPriceRejectsNonUSDAndBadInputs(t *testing.T) {
	_, err := DeriveSellPrice(PriceCost{Currency: domain.PricingCurrencyCNY, InputPerMTok: 2}, 0, 7)
	require.Error(t, err, "人民币成本基不能走换汇派生")

	_, err = DeriveSellPrice(PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 1}, 0, 0)
	require.Error(t, err, "汇率必须为正")

	_, err = DeriveSellPrice(PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 1, DiscountPct: 120}, 0, 7)
	require.Error(t, err, "折扣必须在 0..100")
}

func TestApplyDerivedSellPriceWritesPriorityAndCacheWrite(t *testing.T) {
	base := &ModelPricing{Currency: domain.PricingCurrencyCNY, InputPricePerToken: 1e-6}
	derived := DerivedSellPrice{
		InputPerToken:      5.6e-6,
		CachedPerToken:     0.56e-6,
		OutputPerToken:     8.4e-6,
		CacheWritePerToken: 5.6e-6,
	}
	priced := ApplyDerivedSellPrice(base, derived)
	require.InDelta(t, 5.6e-6, priced.InputPricePerToken, 1e-15)
	require.InDelta(t, 5.6e-6, priced.InputPricePerTokenPriority, 1e-15)
	require.InDelta(t, 8.4e-6, priced.OutputPricePerToken, 1e-15)
	require.InDelta(t, 0.56e-6, priced.CacheReadPricePerToken, 1e-15)
	require.InDelta(t, 5.6e-6, priced.CacheCreationPricePerToken, 1e-15)
	require.True(t, priced.CacheCreationPriceExplicit)
	require.InDelta(t, 1e-6, base.InputPricePerToken, 1e-15, "原对象不得被修改")
}

// DeepSeek 官方峰谷：工作日 01:00-04:00 / 06:00-10:00 UTC 为峰，其余时间（含周末）半价。
func deepSeekOffPeakModifier() PriceModifier {
	return PriceModifier{
		ID:          1,
		Scope:       ModifierScopeVendorTime,
		Target:      "deepseek-v4*",
		Kind:        "window",
		WhenMode:    "outside",
		WindowStart: "01:00",
		WindowEnd:   "04:00",
		TZ:          "UTC",
		Weekdays:    []int{1, 2, 3, 4, 5},
		Multiplier:  0.5,
		Enabled:     true,
	}
}

func TestApplyPriceModifiersVendorOffPeak(t *testing.T) {
	base := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 8e-6, CacheReadPricePerToken: 0.04e-6}
	mods := []PriceModifier{deepSeekOffPeakModifier()}

	// 2026-11-05 是周四；02:00 UTC 落在峰时窗口内 → outside 不命中。
	peak := time.Date(2026, time.November, 5, 2, 0, 0, 0, time.UTC)
	out, applied := ApplyPriceModifiers(base, mods, ModifierContext{Model: "deepseek-v4.1-flash", Now: peak})
	require.InDelta(t, 2e-6, out.InputPricePerToken, 1e-15)
	require.Empty(t, applied)

	// 04:30 UTC 在窗口外 → 半价。
	offPeak := time.Date(2026, time.November, 5, 4, 30, 0, 0, time.UTC)
	out, applied = ApplyPriceModifiers(base, mods, ModifierContext{Model: "deepseek-v4.1-flash", Now: offPeak})
	require.InDelta(t, 1e-6, out.InputPricePerToken, 1e-15)
	require.InDelta(t, 4e-6, out.OutputPricePerToken, 1e-15)
	require.InDelta(t, 0.02e-6, out.CacheReadPricePerToken, 1e-15)
	require.Len(t, applied, 1)
	require.EqualValues(t, 1, applied[0].ID)

	// 周六 02:00 UTC 虽然落在时间窗内，但官方周末无峰时 → 半价。
	saturday := time.Date(2026, time.November, 7, 2, 0, 0, 0, time.UTC)
	out, _ = ApplyPriceModifiers(base, mods, ModifierContext{Model: "deepseek-v4.1-flash", Now: saturday})
	require.InDelta(t, 1e-6, out.InputPricePerToken, 1e-15)
}

func TestApplyPriceModifiersDisabledAndNonMatching(t *testing.T) {
	base := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 8e-6}
	mod := deepSeekOffPeakModifier()
	mod.Enabled = false
	offPeak := time.Date(2026, time.November, 5, 4, 30, 0, 0, time.UTC)
	out, applied := ApplyPriceModifiers(base, []PriceModifier{mod}, ModifierContext{Model: "deepseek-v4.1-flash", Now: offPeak})
	require.InDelta(t, 2e-6, out.InputPricePerToken, 1e-15)
	require.Empty(t, applied)

	// 关闭峰谷后任何时刻都是平价。
	mod.Enabled = true
	mod.Multiplier = 1
	out, _ = ApplyPriceModifiers(base, []PriceModifier{mod}, ModifierContext{Model: "deepseek-v4.1-flash", Now: offPeak})
	require.InDelta(t, 2e-6, out.InputPricePerToken, 1e-15)

	// 模型不匹配 → 不生效。
	out, _ = ApplyPriceModifiers(base, []PriceModifier{deepSeekOffPeakModifier()}, ModifierContext{Model: "glm-5.3", Now: offPeak})
	require.InDelta(t, 2e-6, out.InputPricePerToken, 1e-15)
}

func TestApplyPriceModifiersTierAndGroupOrder(t *testing.T) {
	base := &ModelPricing{InputPricePerToken: 10e-6, OutputPricePerToken: 30e-6}
	mods := []PriceModifier{
		{ID: 10, Scope: ModifierScopeRequestTier, Target: "*", Kind: "tier", TierValue: "flex", Multiplier: 0.5, Enabled: true},
		{ID: 20, Scope: ModifierScopeGroupTime, Target: "49", Kind: "window", Multiplier: 2, Enabled: true},
	}
	ctx := ModifierContext{Model: "hinoki-offensive", GroupID: 49, ServiceTier: "flex", GroupPeakMultiplier: 1.5}

	out, applied := ApplyPriceModifiers(base, mods, ctx)
	// 10 × 0.5（flex）× 2（group_time 记录）× 1.5（分组峰时）= 15
	require.InDelta(t, 15e-6, out.InputPricePerToken, 1e-15)
	require.Len(t, applied, 3)
	require.EqualValues(t, 10, applied[0].ID)
	require.EqualValues(t, 20, applied[1].ID)
	require.EqualValues(t, 0, applied[2].ID, "分组峰时用 0 号合成记录")
}

func TestApplyPriceModifiersCrossMidnightWindow(t *testing.T) {
	mod := PriceModifier{
		ID: 5, Scope: ModifierScopeVendorTime, Target: "night-model", Kind: "window",
		WhenMode: "inside", WindowStart: "22:00", WindowEnd: "02:00", TZ: "UTC", Multiplier: 0.8, Enabled: true,
	}
	base := &ModelPricing{InputPricePerToken: 10e-6}
	inside := time.Date(2026, time.November, 5, 23, 0, 0, 0, time.UTC)
	out, _ := ApplyPriceModifiers(base, []PriceModifier{mod}, ModifierContext{Model: "night-model", Now: inside})
	require.InDelta(t, 8e-6, out.InputPricePerToken, 1e-15)

	outside := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	out, _ = ApplyPriceModifiers(base, []PriceModifier{mod}, ModifierContext{Model: "night-model", Now: outside})
	require.InDelta(t, 10e-6, out.InputPricePerToken, 1e-15)
}

func TestPriceTuplesDiffer(t *testing.T) {
	a := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6}
	b := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6}
	require.False(t, priceTuplesDiffer(a, b))
	b.OutputPricePerToken = 2.001e-6
	require.True(t, priceTuplesDiffer(a, b))
	require.True(t, priceTuplesDiffer(a, nil))
}
