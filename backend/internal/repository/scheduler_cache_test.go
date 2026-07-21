package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerCredentialsKeepsSubscriptionPlanType(t *testing.T) {
	filtered := filterSchedulerCredentials(map[string]any{
		"plan_type":     "plus",
		"access_token":  "secret-access-token",
		"refresh_token": "secret-refresh-token",
	})

	require.Equal(t, "plus", filtered["plan_type"])
	require.NotContains(t, filtered, "access_token")
	require.NotContains(t, filtered, "refresh_token")
}

func TestSchedulerMetadataAccountKeepsOpenAISubscriptionIdentity(t *testing.T) {
	account := service.Account{
		ID:       24,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"plan_type":    "plus",
			"access_token": "secret-access-token",
		},
	}

	metadata := buildSchedulerMetadataAccount(account)

	require.True(t, metadata.IsOpenAIChatGPTSubscription())
	require.Empty(t, metadata.GetCredential("access_token"))
}

func TestBuildSchedulerMetadataAccountKeepsGrokQuotaSnapshots(t *testing.T) {
	billing := map[string]any{
		"usage_percent": 25.0,
		"status_code":   200,
		"fetched_at":    "2026-07-15T09:00:00Z",
	}
	quota := map[string]any{
		"requests":   map[string]any{"limit": 100, "remaining": 75},
		"updated_at": "2026-07-15T09:00:00Z",
	}
	account := service.Account{ID: 89, Platform: service.PlatformGrok, Extra: map[string]any{
		"grok_billing_snapshot": billing,
		"grok_usage_snapshot":   quota,
		"unused_large_field":    "drop-me",
	}}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, billing, got.Extra["grok_billing_snapshot"])
	require.Equal(t, quota, got.Extra["grok_usage_snapshot"])
	require.NotContains(t, got.Extra, "unused_large_field")
}
