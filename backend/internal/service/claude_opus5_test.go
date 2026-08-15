package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	opus5InputPricePerToken         = 5e-6
	opus5OutputPricePerToken        = 25e-6
	opus5CacheCreationPricePerToken = 6.25e-6
	opus5CacheReadPricePerToken     = 0.5e-6
)

func TestClaudeOpus5FamilyFallbackDoesNotUseLegacyOpusRates(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"claude-opus-4-8": {
			InputCostPerToken:           opus5InputPricePerToken,
			OutputCostPerToken:          opus5OutputPricePerToken,
			CacheCreationInputTokenCost: opus5CacheCreationPricePerToken,
			CacheReadInputTokenCost:     opus5CacheReadPricePerToken,
		},
		"claude-opus-4-1":        {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		"claude-3-opus-20240229": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
	}})

	for _, model := range []string{"claude-opus-5", "us.anthropic.claude-opus-5-v1"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.NotNil(t, pricing)
			assert.InDelta(t, opus5InputPricePerToken, pricing.InputPricePerToken, 1e-12)
			assert.InDelta(t, opus5OutputPricePerToken, pricing.OutputPricePerToken, 1e-12)
		})
	}
}

func TestClaudeOpus5HardcodedFallbackPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	for _, model := range []string{"claude-opus-5", "us.anthropic.claude-opus-5-v1", "claude-opus-4-8"} {
		pricing, err := svc.GetModelPricing(model)
		require.NoError(t, err)
		require.NotNil(t, pricing)
		assert.InDelta(t, opus5InputPricePerToken, pricing.InputPricePerToken, 1e-12)
		assert.InDelta(t, opus5OutputPricePerToken, pricing.OutputPricePerToken, 1e-12)
	}

	opus5, err := svc.GetModelPricing("claude-opus-5")
	require.NoError(t, err)
	assert.InDelta(t, opus5CacheCreationPricePerToken, opus5.CacheCreationPricePerToken, 1e-12)
	assert.InDelta(t, opus5CacheReadPricePerToken, opus5.CacheReadPricePerToken, 1e-12)
}

func TestClaudeOpus5Catalog(t *testing.T) {
	assert.Contains(t, claude.DefaultModelIDs(), "claude-opus-5")
}
