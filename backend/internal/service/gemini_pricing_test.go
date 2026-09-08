package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingService_GeminiPricing(t *testing.T) {
	svc := &BillingService{
		fallbackPrices: make(map[string]*ModelPricing),
	}
	svc.initFallbackPricing()

	// Flash models: 3e-7 in, 2.5e-6 out
	for _, m := range []string{"gemini-3.8-flash", "gemini-3.8-flash-high", "gemini-3.7-flash", "gemini-3.6-flash", "models/gemini-3.8-flash"} {
		p, err := svc.GetModelPricing(m)
		require.NoError(t, err, "model %s should have pricing", m)
		require.InDelta(t, 3e-7, p.InputPricePerToken, 1e-12, "model %s input price", m)
		require.InDelta(t, 2.5e-6, p.OutputPricePerToken, 1e-12, "model %s output price", m)
	}

	// Pro models: 2e-6 in, 1.2e-5 out
	for _, m := range []string{"gemini-3.1-pro", "gemini-3.1-pro-high", "gemini-3.1-pro-low", "models/gemini-3.1-pro"} {
		p, err := svc.GetModelPricing(m)
		require.NoError(t, err, "model %s should have pricing", m)
		require.InDelta(t, 2e-6, p.InputPricePerToken, 1e-12, "model %s input price", m)
		require.InDelta(t, 1.2e-5, p.OutputPricePerToken, 1e-12, "model %s output price", m)
	}

	// Fuzzy fallbacks
	pFlash, err := svc.GetModelPricing("gemini-future-flash")
	require.NoError(t, err)
	require.InDelta(t, 3e-7, pFlash.InputPricePerToken, 1e-12)

	pPro, err := svc.GetModelPricing("gemini-future-pro")
	require.NoError(t, err)
	require.InDelta(t, 2e-6, pPro.InputPricePerToken, 1e-12)
}
