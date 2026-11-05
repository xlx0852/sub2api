//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func newRegistryStub(rows map[string]*domain.ModelPriceRow, costs map[int64]*domain.PriceCost) *ChannelService {
	return &ChannelService{
		repo: &mockChannelRepository{
			getModelPriceFn: func(_ context.Context, modelID string, groupID *int64) (*domain.ModelPriceRow, error) {
				if r, ok := rows[modelID]; ok {
					return r, nil
				}
				return nil, nil
			},
		},
		costs:              costs,
		settingsReader:     &stubSettingsReader{value: "model_prices"},
		pricingSourceCache: modelPricingSourceCache{value: "model_prices", loadedAt: time.Now()},
		modifiersLoaded:    true,
	}
}

func regF64(v float64) *float64 { return &v }
func regI64(v int64) *int64     { return &v }

// 注册表价格行：标准价 + priority 价 + 图片价，都要正确落到 ModelPricing。
func TestModelPriceRowToPricingKeepsPriorityAndImage(t *testing.T) {
	svc := &ChannelService{}
	row := &domain.ModelPriceRow{
		ModelID: "gpt-x", Currency: "USD", FxMode: domain.PricingFxModeManual,
		InputPrice: regF64(2e-6), OutputPrice: regF64(8e-6),
		CacheReadPrice: regF64(0.2e-6), CacheWritePrice: regF64(2.5e-6),
		InputPricePriority: regF64(4e-6), OutputPricePriority: regF64(16e-6),
		ImageOutputPrice: regF64(1e-5),
	}
	pricing, err := svc.modelPriceRowToPricing(context.Background(), row)
	require.NoError(t, err)
	require.InDelta(t, 2e-6, pricing.InputPricePerToken, 1e-15)
	require.InDelta(t, 4e-6, pricing.InputPricePerTokenPriority, 1e-15, "priority 价不能被标准价覆盖")
	require.InDelta(t, 16e-6, pricing.OutputPricePerTokenPriority, 1e-15)
	require.InDelta(t, 0.2e-6, pricing.CacheReadPricePerToken, 1e-15)
	require.True(t, pricing.CacheCreationPriceExplicit)
	require.InDelta(t, 1e-5, pricing.ImageOutputPricePerToken, 1e-15)
}

// fx_auto 行：按成本基 × 汇率 × (1+加价) 派生，并覆盖标准/priority 两套价。
func TestModelPriceRowToPricingDerivesFxAuto(t *testing.T) {
	svc := newRegistryStub(nil, map[int64]*domain.PriceCost{
		5: {ID: 5, Vendor: "refuseless", ModelPattern: "glm-5.3-abliterated",
			Currency: domain.PricingCurrencyUSD, InputPerMTok: 1, OutputPerMTok: 2, CachedPerMTok: 0.15, CacheWritePerMTok: 1},
	})
	row := &domain.ModelPriceRow{
		ModelID: "glm-5.3-abliterated", Currency: "CNY",
		FxMode: domain.PricingFxModeAuto, CostID: regI64(5), FxRate: regF64(7), MarkupPct: 0,
		InputPrice: regF64(1e-9), OutputPrice: regF64(1e-9), // 存的是旧值，派生应覆盖
	}
	pricing, err := svc.modelPriceRowToPricing(context.Background(), row)
	require.NoError(t, err)
	require.InDelta(t, 7e-6, pricing.InputPricePerToken, 1e-15)
	require.InDelta(t, 14e-6, pricing.OutputPricePerToken, 1e-15)
	require.InDelta(t, 1.05e-6, pricing.CacheReadPricePerToken, 1e-15)
}

func TestModelPriceRowToPricingFailsWhenCostMissing(t *testing.T) {
	svc := newRegistryStub(nil, map[int64]*domain.PriceCost{})
	row := &domain.ModelPriceRow{
		ModelID: "m", FxMode: domain.PricingFxModeAuto, CostID: regI64(99), FxRate: regF64(7),
	}
	_, err := svc.modelPriceRowToPricing(context.Background(), row)
	require.ErrorIs(t, err, ErrPriceCostMissing)
}

func TestNormalizeModelPricingSource(t *testing.T) {
	require.Equal(t, PricingModelSourceLegacy, normalizeModelPricingSource(""))
	require.Equal(t, PricingModelSourceDual, normalizeModelPricingSource("DUAL"))
	require.Equal(t, PricingModelSourceRegistry, normalizeModelPricingSource("model_prices"))
	require.Equal(t, PricingModelSourceRegistry, normalizeModelPricingSource("registry"))
	require.Equal(t, PricingModelSourceLegacy, normalizeModelPricingSource("nonsense"))
}

// 注册表来源下：命中行 → 覆盖；未命中 → fail-closed（BasePricing=nil）。
func TestApplyRegistryPricingOverridesAndFailsClosed(t *testing.T) {
	svc := newRegistryStub(map[string]*domain.ModelPriceRow{
		"hinoki-offensive": {ModelID: "hinoki-offensive", Currency: "CNY", FxMode: domain.PricingFxModeManual,
			InputPrice: regF64(5.6e-6), OutputPrice: regF64(8.4e-6)},
	}, nil)
	resolver := &ModelPricingResolver{channelService: svc}
	groupID := int64(49)

	resolved := &ResolvedPricing{Mode: BillingModeToken, Currency: domain.PricingCurrencyUSD,
		BasePricing: &ModelPricing{InputPricePerToken: 1e-6}}
	resolver.applyRegistryPricing(context.Background(), PricingInput{Model: "hinoki-offensive", GroupID: &groupID}, resolved)
	require.Equal(t, PricingSourceRegistry, resolved.Source)
	require.InDelta(t, 5.6e-6, resolved.BasePricing.InputPricePerToken, 1e-15)

	missing := &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{InputPricePerToken: 1e-6}}
	resolver.applyRegistryPricing(context.Background(), PricingInput{Model: "unknown-model", GroupID: &groupID}, missing)
	require.Nil(t, missing.BasePricing, "注册表里没有价时必须 fail-closed")
}

// legacy 来源：完全不动现有结果。
func TestApplyRegistryPricingLegacyIsNoop(t *testing.T) {
	svc := newRegistryStub(nil, nil)
	svc.pricingSourceCache = modelPricingSourceCache{value: "legacy", loadedAt: time.Now()}
	resolver := &ModelPricingResolver{channelService: svc}
	groupID := int64(49)
	resolved := &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{InputPricePerToken: 1e-6}}
	resolver.applyRegistryPricing(context.Background(), PricingInput{Model: "hinoki-offensive", GroupID: &groupID}, resolved)
	require.InDelta(t, 1e-6, resolved.BasePricing.InputPricePerToken, 1e-15)
}

// dual 模式：注册表价格与现有链路不同时不得改扣费，只打点。
func TestApplyRegistryPricingDualDoesNotOverride(t *testing.T) {
	svc := newRegistryStub(map[string]*domain.ModelPriceRow{
		"grok-4.7": {ModelID: "grok-4.7", Currency: "USD", FxMode: domain.PricingFxModeManual,
			InputPrice: regF64(3e-6), OutputPrice: regF64(9e-6)},
	}, nil)
	svc.pricingSourceCache = modelPricingSourceCache{value: "dual", loadedAt: time.Now()}
	resolver := &ModelPricingResolver{channelService: svc}
	groupID := int64(23)
	legacy := &ResolvedPricing{Mode: BillingModeToken, Currency: domain.PricingCurrencyUSD,
		Source: PricingSourceLiteLLM, BasePricing: &ModelPricing{InputPricePerToken: 2e-6}}
	resolver.applyRegistryPricing(context.Background(), PricingInput{Model: "grok-4.7", GroupID: &groupID}, legacy)
	require.Equal(t, PricingSourceLiteLLM, legacy.Source, "dual 不得改来源")
	require.InDelta(t, 2e-6, legacy.BasePricing.InputPricePerToken, 1e-15, "dual 不得改扣费价格")
}
