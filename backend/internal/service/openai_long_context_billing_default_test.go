package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBillingService_GPT56LongContextExactThresholdAndOneTokenAbove(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	atThreshold := UsageTokens{
		InputTokens:         100_000,
		CacheCreationTokens: 100_000,
		CacheReadTokens:     72_000,
		OutputTokens:        10,
	}
	base, err := svc.CalculateCost("gpt-5.6-sol", atThreshold, 1)
	require.NoError(t, err)
	require.InDelta(t, 100_000*4e-6, base.InputCost, 1e-12)
	require.InDelta(t, 100_000*5e-6, base.CacheCreationCost, 1e-12)
	require.InDelta(t, 72_000*0.4e-6, base.CacheReadCost, 1e-12)
	require.InDelta(t, 10*20e-6, base.OutputCost, 1e-12)

	aboveThreshold := atThreshold
	aboveThreshold.CacheReadTokens++
	longContext, err := svc.CalculateCost("gpt-5.6-sol", aboveThreshold, 1)
	require.NoError(t, err)
	require.InDelta(t, 100_000*4e-6*2, longContext.InputCost, 1e-12)
	require.InDelta(t, 100_000*5e-6*2, longContext.CacheCreationCost, 1e-12)
	require.InDelta(t, 72_001*0.4e-6*2, longContext.CacheReadCost, 1e-12)
	require.InDelta(t, 10*20e-6*1.5, longContext.OutputCost, 1e-12)
}
