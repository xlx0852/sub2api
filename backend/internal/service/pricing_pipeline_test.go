//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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

// DeepSeek 官方峰谷的正式配置是"两条峰时窗口 + 一条兜底"，必须首个命中生效，
// 否则 0.5 会被两条窗口各乘一次变成 0.25。
func TestApplyPriceModifiersDeepSeekSeedShape(t *testing.T) {
	peakWindows := []PriceModifier{
		{
			ID: 1, Scope: ModifierScopeVendorTime, Target: "deepseek-v4*", Kind: "window",
			WhenMode: "inside", WindowStart: "01:00", WindowEnd: "04:00", TZ: "UTC",
			Weekdays: []int{1, 2, 3, 4, 5}, Multiplier: 1.0, Enabled: true, Priority: 10,
		},
		{
			ID: 2, Scope: ModifierScopeVendorTime, Target: "deepseek-v4*", Kind: "window",
			WhenMode: "inside", WindowStart: "06:00", WindowEnd: "10:00", TZ: "UTC",
			Weekdays: []int{1, 2, 3, 4, 5}, Multiplier: 1.0, Enabled: true, Priority: 10,
		},
		{
			ID: 3, Scope: ModifierScopeVendorTime, Target: "deepseek-v4*", Kind: "window",
			WhenMode: "inside", Multiplier: 0.5, Enabled: true, Priority: 100,
		},
	}
	base := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 8e-6}

	cases := []struct {
		name string
		now  time.Time
		want float64
	}{
		{"峰时 02:00 UTC 周四", time.Date(2026, time.November, 5, 2, 0, 0, 0, time.UTC), 2e-6},
		{"峰时 07:00 UTC 周四", time.Date(2026, time.November, 5, 7, 0, 0, 0, time.UTC), 2e-6},
		{"谷时 04:30 UTC 周四", time.Date(2026, time.November, 5, 4, 30, 0, 0, time.UTC), 1e-6},
		{"周六 02:00 UTC 无峰时", time.Date(2026, time.November, 7, 2, 0, 0, 0, time.UTC), 1e-6},
	}
	for _, tc := range cases {
		out, applied := ApplyPriceModifiers(base, peakWindows, ModifierContext{Model: "deepseek-v4.1-flash", Now: tc.now})
		require.InDelta(t, tc.want, out.InputPricePerToken, 1e-15, tc.name)
		require.InDelta(t, tc.want*4, out.OutputPricePerToken, 1e-15, tc.name)
		require.Len(t, applied, 1, "%s: 每次只应有一个时间窗因子生效", tc.name)
	}
}

func TestPipelineModeReadsConfig(t *testing.T) {
	legacy := NewBillingService(&config.Config{}, nil)
	require.Equal(t, PricingPipelineLegacy, legacy.resolvePipelineMode(context.Background()), "默认必须是 legacy")

	cfg := &config.Config{}
	cfg.Pricing.Pipeline = "shadow"
	require.Equal(t, PricingPipelineShadow, NewBillingService(cfg, nil).resolvePipelineMode(context.Background()))

	cfg.Pricing.Pipeline = "UNIFIED"
	require.Equal(t, PricingPipelineUnified, NewBillingService(cfg, nil).resolvePipelineMode(context.Background()))

	cfg.Pricing.Pipeline = "nonsense"
	require.Equal(t, PricingPipelineLegacy, NewBillingService(cfg, nil).resolvePipelineMode(context.Background()))
}

func newStubChannelService(mods []PriceModifier, costs map[int64]*PriceCost) *ChannelService {
	return &ChannelService{
		modifiers:       mods,
		modifiersLoaded: true,
		modifiersAtTime: time.Now(),
		costs:           costs,
	}
}

// fx_auto 行 + 成本基 → 统一管线按汇率派生，并叠加 vendor_time 谷时折扣。
func TestUnifiedTokenPriceDerivesAndAppliesModifiers(t *testing.T) {
	rate := 7.0
	costID := int64(11)
	svc := newStubChannelService(
		[]PriceModifier{
			{ID: 1, Scope: ModifierScopeVendorTime, Target: "hinoki-*", Kind: "window", WhenMode: "inside", Multiplier: 0.5, Enabled: true, Priority: 10},
		},
		map[int64]*PriceCost{
			11: {ID: 11, Vendor: "hinoki", ModelPattern: "hinoki-offensive", Currency: domain.PricingCurrencyUSD,
				InputPerMTok: 0.80, CachedPerMTok: 0.08, OutputPerMTok: 1.20, CacheWritePerMTok: 0.80},
		},
	)
	resolver := &ModelPricingResolver{channelService: svc}

	resolved := &ResolvedPricing{
		Mode:     BillingModeToken,
		Currency: domain.PricingCurrencyCNY,
		Source:   PricingSourceChannel,
		channelPricing: &domain.ChannelModelPricing{
			FxMode:    domain.PricingFxModeAuto,
			CostID:    &costID,
			FxRate:    &rate,
			MarkupPct: 0,
		},
	}
	base := &ModelPricing{Currency: domain.PricingCurrencyCNY, InputPricePerToken: 5.6e-6, OutputPricePerToken: 8.4e-6}

	out, applied, ok := resolver.UnifiedTokenPrice(context.Background(), resolved, UnifiedPriceRequest{
		Model: "hinoki-offensive", GroupID: 49, Now: time.Now(), Base: base,
	})
	require.True(t, ok)
	// 5.6e-6 × 0.5 的窗口因子
	require.InDelta(t, 2.8e-6, out.InputPricePerToken, 1e-15)
	require.InDelta(t, 4.2e-6, out.OutputPricePerToken, 1e-15)
	require.Len(t, applied, 1)
}

// fx_auto 但缺成本基 / 缺汇率 → 必须放弃统一管线，让调用方保持 legacy 结果。
func TestUnifiedTokenPriceFailsClosedWhenCostOrRateMissing(t *testing.T) {
	rate := 7.0
	costID := int64(99)
	svc := newStubChannelService(nil, map[int64]*PriceCost{})
	resolver := &ModelPricingResolver{channelService: svc}
	base := &ModelPricing{InputPricePerToken: 1e-6}

	missingCost := &ResolvedPricing{Mode: BillingModeToken, channelPricing: &domain.ChannelModelPricing{
		FxMode: domain.PricingFxModeAuto, CostID: &costID, FxRate: &rate,
	}}
	_, _, ok := resolver.UnifiedTokenPrice(context.Background(), missingCost, UnifiedPriceRequest{Model: "m", Base: base})
	require.False(t, ok, "成本基缺失时必须返回 false")

	missingRate := &ResolvedPricing{Mode: BillingModeToken, channelPricing: &domain.ChannelModelPricing{
		FxMode: domain.PricingFxModeAuto, CostID: &costID,
	}}
	_, _, ok = resolver.UnifiedTokenPrice(context.Background(), missingRate, UnifiedPriceRequest{Model: "m", Base: base})
	require.False(t, ok, "汇率缺失时必须返回 false")
}

func TestPipelineSkipsLegacyTimeOfDay(t *testing.T) {
	require.False(t, pipelineSkipsLegacyTimeOfDay("legacy"))
	require.False(t, pipelineSkipsLegacyTimeOfDay("shadow"), "shadow 仍按 legacy 扣费，谷时折扣照旧")
	require.True(t, pipelineSkipsLegacyTimeOfDay("unified"))
}

func TestHasVendorTimeModifier(t *testing.T) {
	mods := []PriceModifier{
		{ID: 1, Scope: ModifierScopeVendorTime, Target: "deepseek-v4*", Enabled: true},
		{ID: 2, Scope: ModifierScopeRequestTier, Target: "*", Enabled: true},
	}
	require.True(t, hasVendorTimeModifier(mods, "deepseek-v4.1-flash"))
	require.False(t, hasVendorTimeModifier(mods, "glm-5.3"))
	mods[0].Enabled = false
	require.False(t, hasVendorTimeModifier(mods, "deepseek-v4.1-flash"))
}

// 线上 8 行的真实值（migration 213 回填后的 CNY 售价，单位 元/百万 token）。
// 该用例锁住"SQL 迁移里的 ROUND 计算"与"Go 派生"结果一致，防止两边算法漂移。
func TestDeriveSellPriceMatchesLivePriceRows(t *testing.T) {
	cases := []struct {
		model     string
		cost      PriceCost
		in, out   float64
		cacheRead float64
	}{
		{
			model: "deepseek-v4-1-flash-1017-uncensored",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.33, CachedPerMTok: 0.00825, OutputPerMTok: 1.32, CacheWritePerMTok: 0.33},
			in:    2.31, out: 9.24, cacheRead: 0.05775,
		},
		{
			model: "deepseek-v4.1-flash-abliterated",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.14, CachedPerMTok: 0.035, OutputPerMTok: 0.50, CacheWritePerMTok: 0.14},
			in:    0.98, out: 3.50, cacheRead: 0.245,
		},
		{
			model: "deepseek-v4.1-flash-fast",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.01, CachedPerMTok: 0.002, OutputPerMTok: 0.08, CacheWritePerMTok: 0.01},
			in:    0.07, out: 0.56, cacheRead: 0.014,
		},
		{
			model: "glm-5.3-abliterated",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 1.00, CachedPerMTok: 0.15, OutputPerMTok: 2.00, CacheWritePerMTok: 1.00},
			in:    7.00, out: 14.00, cacheRead: 1.05,
		},
		{
			model: "glm-5.3-flash-abliterated",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.12, CachedPerMTok: 0.04, OutputPerMTok: 0.50, CacheWritePerMTok: 0.12},
			in:    0.84, out: 3.50, cacheRead: 0.28,
		},
		{
			model: "minimax-m3.1-flash",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.02, CachedPerMTok: 0.002, OutputPerMTok: 0.08, CacheWritePerMTok: 0.02},
			in:    0.14, out: 0.56, cacheRead: 0.014,
		},
		{
			model: "hinoki-offensive",
			cost:  PriceCost{Currency: domain.PricingCurrencyUSD, InputPerMTok: 0.80, CachedPerMTok: 0.08, OutputPerMTok: 1.20, CacheWritePerMTok: 0.80},
			in:    5.60, out: 8.40, cacheRead: 0.56,
		},
	}
	for _, tc := range cases {
		derived, err := DeriveSellPrice(tc.cost, 0, 7.0)
		require.NoError(t, err, tc.model)
		require.InDelta(t, tc.in, derived.InputPerToken*1e6, 1e-9, "%s 输入价", tc.model)
		require.InDelta(t, tc.out, derived.OutputPerToken*1e6, 1e-9, "%s 输出价", tc.model)
		require.InDelta(t, tc.cacheRead, derived.CachedPerToken*1e6, 1e-9, "%s 缓存价", tc.model)
	}
}

type stubSettingsReader struct {
	value string
	err   error
}

func (s *stubSettingsReader) GetRawValue(ctx context.Context, key string) (string, error) {
	return s.value, s.err
}

// 设置项优先于配置文件；空值回落配置；读失败保留上一次结果。
func TestResolvePipelineModePrefersSettings(t *testing.T) {
	cfg := &config.Config{}
	cfg.Pricing.Pipeline = "legacy"
	svc := NewBillingService(cfg, nil)

	require.Equal(t, PricingPipelineLegacy, svc.resolvePipelineMode(context.Background()), "无设置读取器时用配置")

	reader := &stubSettingsReader{value: "shadow"}
	svc.SetSettingsReader(reader)
	require.Equal(t, PricingPipelineShadow, svc.resolvePipelineMode(context.Background()), "设置项优先")

	// 设置项清空 → 回落配置
	reader.value = ""
	svc.pipelineCache.loadedAt = time.Time{} // 绕过 10s 缓存
	require.Equal(t, PricingPipelineLegacy, svc.resolvePipelineMode(context.Background()))

	// 设置项改为 unified
	reader.value = "unified"
	svc.pipelineCache.loadedAt = time.Time{}
	require.Equal(t, PricingPipelineUnified, svc.resolvePipelineMode(context.Background()))

	// 读失败 → 保持上一次的 unified，不静默回退
	reader.err = errStubSettings
	svc.pipelineCache.loadedAt = time.Time{}
	require.Equal(t, PricingPipelineUnified, svc.resolvePipelineMode(context.Background()))

	// 未知值 → legacy
	reader.err = nil
	reader.value = "garbage"
	svc.pipelineCache.loadedAt = time.Time{}
	require.Equal(t, PricingPipelineLegacy, svc.resolvePipelineMode(context.Background()))
}

var errStubSettings = errorsNew("settings unavailable")

func errorsNew(msg string) error { return &stubErr{msg} }

type stubErr struct{ msg string }

func (e *stubErr) Error() string { return e.msg }
