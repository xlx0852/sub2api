package service

import "math"

// roundUsagePercent2 snaps a 0–100 quota used% to two decimal places.
// NaN/Inf are returned unchanged so callers can keep their existing guards.
func roundUsagePercent2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return math.Round(v*100) / 100
}

func roundUsagePercent2Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := roundUsagePercent2(*v)
	return &r
}

// roundUsageRatio2 snaps a 0–1 utilization ratio so the equivalent
// percentage has two decimal places (0.12345 → 0.1235 = 12.35%).
func roundUsageRatio2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return roundUsagePercent2(v*100) / 100
}

func roundOpenAIQuotaUsagePercents(usage *OpenAIQuotaUsage) {
	if usage == nil {
		return
	}
	roundWindow := func(w *OpenAIRateLimitWindow) {
		if w == nil {
			return
		}
		w.UsedPercent = roundUsagePercent2(w.UsedPercent)
	}
	roundRateLimit := func(rl *OpenAIRateLimit) {
		if rl == nil {
			return
		}
		roundWindow(rl.PrimaryWindow)
		roundWindow(rl.SecondaryWindow)
	}
	roundRateLimit(usage.RateLimit)
	for i := range usage.AdditionalRateLimits {
		roundRateLimit(usage.AdditionalRateLimits[i].RateLimit)
	}
}
