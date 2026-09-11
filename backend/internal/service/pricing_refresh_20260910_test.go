package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekV41FlashBillingPeakAndOffPeak(t *testing.T) {
	for _, model := range []string{"deepseek-flash", "deepseek-v4.1-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		t.Run(model, func(t *testing.T) {
			svc := NewBillingService(&config.Config{}, nil)
			svc.now = func() time.Time { return time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC) }
			usage := UsageTokens{InputTokens: 1000000, OutputTokens: 1000000, CacheReadTokens: 1000000}
			peak, err := svc.CalculateCost(model, usage, 1)
			require.NoError(t, err)
			require.Equal(t, "CNY", string(peak.PricingCurrency))
			require.InDelta(t, 2, peak.InputCost, 1e-9)
			require.InDelta(t, 8, peak.OutputCost, 1e-9)
			require.InDelta(t, .04, peak.CacheReadCost, 1e-9)
			svc.now = func() time.Time { return time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC) }
			off, err := svc.CalculateCost(model, usage, 1)
			require.NoError(t, err)
			require.InDelta(t, peak.TotalCost*.5, off.TotalCost, 1e-9)
		})
	}
}
