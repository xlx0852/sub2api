package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAntigravitySubscription(t *testing.T) {
	t.Run("nil response returns Free", func(t *testing.T) {
		res := NormalizeAntigravitySubscription(nil)
		require.Equal(t, "Free", res.PlanType)
		require.Empty(t, res.SubscriptionStatus)
	})

	t.Run("Account 118: paid Pro with ineligible free-tier should NOT be Abnormal", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			PaidTier: &antigravity.PaidTierInfo{
				ID:   "g1-pro-tier",
				Name: "Google AI Pro",
			},
			CurrentTier: &antigravity.TierInfo{
				ID:   "standard-tier",
				Name: "Antigravity",
			},
			IneligibleTiers: []*antigravity.IneligibleTier{
				{
					TierID:        "free-tier",
					TierName:      "Antigravity",
					ReasonCode:    "UNSUPPORTED_LOCATION",
					ReasonMessage: "Your current account is not eligible for Antigravity, because it is not currently available in your location.",
				},
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Pro", res.PlanType)
		require.Empty(t, res.SubscriptionStatus)
		require.Empty(t, res.SubscriptionError)
	})

	t.Run("Account 115: paid Pro with current free-tier should be Pro", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			PaidTier: &antigravity.PaidTierInfo{
				ID:   "g1-pro-tier",
				Name: "Google AI Pro",
			},
			CurrentTier: &antigravity.TierInfo{
				ID:   "free-tier",
				Name: "Antigravity",
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Pro", res.PlanType)
		require.Empty(t, res.SubscriptionStatus)
	})

	t.Run("Normal free account should be Free", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			CurrentTier: &antigravity.TierInfo{
				ID:   "free-tier",
				Name: "Antigravity",
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Free", res.PlanType)
		require.Empty(t, res.SubscriptionStatus)
	})

	t.Run("Free account blocked by location should be Abnormal", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			CurrentTier: &antigravity.TierInfo{
				ID:   "free-tier",
				Name: "Antigravity",
			},
			IneligibleTiers: []*antigravity.IneligibleTier{
				{
					TierID:        "free-tier",
					ReasonCode:    "UNSUPPORTED_LOCATION",
					ReasonMessage: "Location blocked",
				},
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Abnormal", res.PlanType)
		require.Equal(t, "abnormal", res.SubscriptionStatus)
		require.Equal(t, "Location blocked", res.SubscriptionError)
	})

	t.Run("Paid account with paid tier ineligible should be Abnormal", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			PaidTier: &antigravity.PaidTierInfo{
				ID:   "g1-pro-tier",
				Name: "Google AI Pro",
			},
			IneligibleTiers: []*antigravity.IneligibleTier{
				{
					TierID:        "g1-pro-tier",
					ReasonCode:    "BILLING_ERROR",
					ReasonMessage: "Subscription payment failed",
				},
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Abnormal", res.PlanType)
		require.Equal(t, "abnormal", res.SubscriptionStatus)
		require.Equal(t, "Subscription payment failed", res.SubscriptionError)
	})

	t.Run("Legacy nested Tier struct in IneligibleTier supported", func(t *testing.T) {
		resp := &antigravity.LoadCodeAssistResponse{
			CurrentTier: &antigravity.TierInfo{
				ID: "free-tier",
			},
			IneligibleTiers: []*antigravity.IneligibleTier{
				{
					Tier: &antigravity.TierInfo{
						ID: "free-tier",
					},
					ReasonMessage: "Legacy error",
				},
			},
		}
		res := NormalizeAntigravitySubscription(resp)
		require.Equal(t, "Abnormal", res.PlanType)
		require.Equal(t, "Legacy error", res.SubscriptionError)
	})
}
