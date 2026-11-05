package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildSellSourceFromChannel(t *testing.T) {
	t.Parallel()

	official := buildSellSourceFromChannel(nil, true)
	require.Equal(t, SellPriceSourceOfficial, official.Source)
	require.False(t, official.Effective)

	in := 0.0000008
	out := 0.0000048
	ch := &Channel{
		ID:     2,
		Name:   "luna-x4",
		Status: StatusActive,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-5.6-luna", "gpt-5.4"}, InputPrice: &in, OutputPrice: &out},
		},
	}
	src := buildSellSourceFromChannel(ch, true)
	require.Equal(t, SellPriceSourcePolicy, src.Source)
	require.True(t, src.Effective)
	require.NotNil(t, src.PolicyID)
	require.Equal(t, int64(2), *src.PolicyID)
	require.Equal(t, "luna-x4", src.PolicyName)
	require.Equal(t, 2, src.ModelCount)
	require.Contains(t, src.SampleModels, "gpt-5.6-luna")

	inactive := buildSellSourceFromChannel(&Channel{ID: 3, Name: "off", Status: StatusDisabled}, false)
	require.Equal(t, SellPriceSourcePolicy, inactive.Source)
	require.False(t, inactive.Effective)
	require.True(t, inactive.InactivePolicy)
}

func TestBuildModelPreviews_MarkupAndEffective(t *testing.T) {
	t.Parallel()

	// official-like base is not injected here; sell-only path
	sellIn := 0.0000008  // $0.80 / 1M
	sellOut := 0.0000048 // $4.80 / 1M
	policy := &Channel{
		ID:     2,
		Status: StatusActive,
		ModelPricing: []ChannelModelPricing{
			{
				Platform:    PlatformOpenAI,
				Models:      []string{"gpt-5.6-luna"},
				BillingMode: BillingModeToken,
				InputPrice:  &sellIn,
				OutputPrice: &sellOut,
			},
			{
				Platform:    PlatformAnthropic, // should be skipped for openai group
				Models:      []string{"claude-sonnet-4"},
				BillingMode: BillingModeToken,
				InputPrice:  &sellIn,
			},
		},
	}
	group := &Group{ID: 4, Platform: PlatformOpenAI, RateMultiplier: 1.5}
	previews := buildModelPreviews(policy, group, nil, true)
	require.Len(t, previews, 1)
	p := previews[0]
	require.Equal(t, "gpt-5.6-luna", p.Model)
	require.Equal(t, SellPriceSourcePolicy, p.Source)
	require.InDelta(t, 0.80, *p.SellInput, 1e-9)
	require.InDelta(t, 4.80, *p.SellOutput, 1e-9)
	require.InDelta(t, 1.20, *p.EffectiveInput, 1e-9) // 0.80 * 1.5
	require.InDelta(t, 7.20, *p.EffectiveOutput, 1e-9)
	// No official → no markup_n
	require.Nil(t, p.MarkupN)
}

func TestBindGroupSellPricePolicy_UnbindNoop(t *testing.T) {
	t.Parallel()
	// nil service methods need repo; just validate input guard
	s := &ChannelService{}
	err := s.BindGroupSellPricePolicy(context.Background(), 0, nil)
	require.Error(t, err)
}

func TestSampleAndCountPricingModels(t *testing.T) {
	t.Parallel()
	pricing := []ChannelModelPricing{
		{Models: []string{"a", "B", "a"}},
		{Models: []string{"b", "c", "d"}},
	}
	require.Equal(t, 6, countPricingModels(pricing))
	samples := samplePricingModels(pricing, 3)
	require.Len(t, samples, 3)
	// case-insensitive dedupe of "a"/"A" not applied across different casing in count,
	// but sample dedupes by lower
	require.Equal(t, "a", samples[0])
}

func TestPopulateChannelCachePrefersGroupToPolicyMap(t *testing.T) {
	t.Parallel()

	in := 0.0000008
	policy := Channel{
		ID:     2,
		Name:   "luna-x4",
		Status: StatusActive,
		// Intentionally empty GroupIDs — explicit policy map is the source of truth.
		GroupIDs: nil,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-5.6-luna"}, InputPrice: &in},
		},
	}
	// channel_groups still has another group for legacy fallback path
	legacy := Channel{
		ID:       9,
		Name:     "legacy",
		Status:   StatusActive,
		GroupIDs: []int64{99},
	}

	groupToPolicy := map[int64]int64{4: 2}
	platforms := map[int64]string{4: PlatformOpenAI, 99: PlatformOpenAI}
	cache := populateChannelCache([]Channel{policy, legacy}, platforms, groupToPolicy)

	require.NotNil(t, cache.channelByGroupID[4])
	require.Equal(t, int64(2), cache.channelByGroupID[4].ID)
	// When explicit map is present, legacy GroupIDs expansion is skipped (P4.1a).
	_, hasLegacy := cache.channelByGroupID[99]
	require.False(t, hasLegacy)

	// Empty map falls back to GroupIDs expansion (pre-migration safety).
	cache2 := populateChannelCache([]Channel{policy, legacy}, platforms, map[int64]int64{})
	require.NotNil(t, cache2.channelByGroupID[99])
	require.Equal(t, int64(9), cache2.channelByGroupID[99].ID)
}

// fx_auto 行必须在后台显示成本与毛利：这里用 hinoki 的真实数字（$0.80 → ¥5.60 @7.0）。
func TestBuildModelPreviews_FxAutoShowsCostAndMargin(t *testing.T) {
	rate := 7.0
	sellIn := 5.6e-6
	sellOut := 8.4e-6
	group := &Group{ID: 49, Name: "国模abliterated", Platform: "openai", RateMultiplier: 1.5}
	policy := &Channel{
		ModelPricing: []ChannelModelPricing{
			{
				Platform:    "openai",
				Models:      []string{"hinoki-offensive"},
				BillingMode: BillingModeToken,
				Currency:    domain.PricingCurrencyCNY,
				InputPrice:  &sellIn,
				OutputPrice: &sellOut,
				FxMode:      domain.PricingFxModeAuto,
				FxRate:      &rate,
				MarkupPct:   0,
			},
		},
	}

	previews := buildModelPreviews(policy, group, nil, true)
	require.Len(t, previews, 1)
	p := previews[0]
	require.Equal(t, domain.PricingFxModeAuto, p.FxMode)
	require.NotNil(t, p.CostInputUSD)
	require.InDelta(t, 0.80, *p.CostInputUSD, 1e-9)
	require.NotNil(t, p.CostOutputUSD)
	require.InDelta(t, 1.20, *p.CostOutputUSD, 1e-9)
	require.NotNil(t, p.GrossMarginPct)
	// 售价 5.6、实付 8.4、成本 5.6 → 毛利 33.3%
	require.InDelta(t, 33.3, *p.GrossMarginPct, 0.1)
}

// 售价表总览：模型清单容器（无价行）要能区分出来，示例价要乘上绑定分组的折扣。
func TestBuildPricingOverview(t *testing.T) {
	sellIn := 5.6e-6
	nullRow := ChannelModelPricing{Platform: "openai", Models: []string{"a", "b"}, BillingMode: BillingModeToken, Currency: domain.PricingCurrencyUSD}
	pricedRow := ChannelModelPricing{Platform: "openai", Models: []string{"hinoki-offensive"}, BillingMode: BillingModeToken,
		Currency: domain.PricingCurrencyCNY, InputPrice: &sellIn, FxMode: domain.PricingFxModeAuto}

	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) {
			return []Channel{
				{ID: 8, Name: "国模", Status: "active", ModelPricing: []ChannelModelPricing{pricedRow}},
				{ID: 5, Name: "Grok", Status: "active", ModelPricing: []ChannelModelPricing{nullRow}},
			}, nil
		},
		listGroupRefsFn: func(ctx context.Context) (map[int64][]domain.GroupPolicyBinding, error) {
			return map[int64][]domain.GroupPolicyBinding{
				8: {{GroupID: 49, Name: "国模abliterated", RateMultiplier: 1.5}},
			}, nil
		},
	}
	svc := NewChannelService(repo, nil, nil, nil)
	out, err := svc.BuildPricingOverview(context.Background())
	require.NoError(t, err)
	require.Len(t, out.Policies, 2)

	// 有绑定+有定价的排前面
	first := out.Policies[0]
	require.Equal(t, int64(8), first.ID)
	require.Equal(t, 1, first.PricedRowCount)
	require.InDelta(t, 5.6, *first.ExampleSellInput, 1e-9)
	require.InDelta(t, 8.4, *first.ExampleEffectiveInput, 1e-9, "示例实付要乘分组折扣")
	require.Equal(t, "hinoki-offensive", first.ExampleModel)
	require.Len(t, first.BoundGroups, 1)

	second := out.Policies[1]
	require.Equal(t, 0, second.PricedRowCount, "无价行只是模型清单容器")
	require.Equal(t, 2, second.ModelCount)
}
