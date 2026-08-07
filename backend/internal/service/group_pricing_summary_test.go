package service

import (
	"context"
	"testing"

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
		// Intentionally empty GroupIDs — P2 map is the source of truth.
		GroupIDs: nil,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-5.6-luna"}, InputPrice: &in},
		},
	}
	// channel_groups still has another group for legacy path
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
	// When explicit map is present, legacy GroupIDs expansion is skipped.
	_, hasLegacy := cache.channelByGroupID[99]
	require.False(t, hasLegacy)

	// Empty map falls back to GroupIDs expansion.
	cache2 := populateChannelCache([]Channel{policy, legacy}, platforms, nil)
	require.NotNil(t, cache2.channelByGroupID[99])
	require.Equal(t, int64(9), cache2.channelByGroupID[99].ID)
}
