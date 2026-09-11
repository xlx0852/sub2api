package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelPricing_CursorGatewayModelNormalization(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	for _, model := range []string{
		"cursor-grok-4.6-low",
		"cursor-grok-4.6-low-fast",
		"cursor-grok-4.6-medium",
		"cursor-grok-4.6-high",
		"cursor-grok-4.6-xhigh",
		"cursor-grok-4.6-xhigh-fast",
	} {
		model := model
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 2e-6, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, 6e-6, pricing.OutputPricePerToken, 1e-12)
		})
	}
}

func TestNormalizeCursorBillingModel(t *testing.T) {
	cases := map[string]string{
		"cursor-grok-4.6-low":       "grok-4.6",
		"cursor-grok-4.6-low-fast":  "grok-4.6",
		"cursor-grok-4.6-xhigh":     "grok-4.6",
		"cursor-claude-opus-5-high": "claude-opus-5",
		"cursor-composer-2.5":       "composer-2.5",
		"cursor-grok-4.6":           "grok-4.6",
	}
	for in, want := range cases {
		assert.Equal(t, want, normalizeCursorBillingModel(in), "model %s", in)
	}
}
