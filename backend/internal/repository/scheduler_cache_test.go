package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerCredentialsKeepsSubscriptionPlanType(t *testing.T) {
	filtered := filterSchedulerCredentials(map[string]any{
		"plan_type":     "plus",
		"access_token":  "secret-access-token",
		"refresh_token": "secret-refresh-token",
		"base_url":      "http://sub2api-devin-bridge:8080",
	})

	require.Equal(t, "plus", filtered["plan_type"])
	require.Equal(t, "http://sub2api-devin-bridge:8080", filtered["base_url"])
	require.NotContains(t, filtered, "access_token")
	require.NotContains(t, filtered, "refresh_token")
}

func TestBuildSchedulerMetadataAccountKeepsExternalBridgeFlags(t *testing.T) {
	account := domain.Account{
		ID:       126,
		Platform: domain.PlatformOpenAI,
		Type:     domain.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "bridge-key",
			"base_url": "http://sub2api-devin-bridge:8080",
		},
		Extra: map[string]any{
			"external_bridge":      true,
			"external_bridge_kind": "devin",
			"openai_responses_mode": "force_responses",
			"unused_large_field":    "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)
	require.Equal(t, true, got.Extra["external_bridge"])
	require.Equal(t, "devin", got.Extra["external_bridge_kind"])
	require.Equal(t, "force_responses", got.Extra["openai_responses_mode"])
	require.Equal(t, "http://sub2api-devin-bridge:8080", got.Credentials["base_url"])
	require.NotContains(t, got.Extra, "unused_large_field")
	require.True(t, got.IsExternalBridge())
}

func TestSchedulerMetadataAccountKeepsOpenAISubscriptionIdentity(t *testing.T) {
	account := domain.Account{
		ID:       24,
		Platform: domain.PlatformOpenAI,
		Type:     domain.AccountTypeOAuth,
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
	account := domain.Account{ID: 89, Platform: domain.PlatformGrok, Extra: map[string]any{
		"grok_billing_snapshot": billing,
		"grok_usage_snapshot":   quota,
		"unused_large_field":    "drop-me",
	}}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, billing, got.Extra["grok_billing_snapshot"])
	require.Equal(t, quota, got.Extra["grok_usage_snapshot"])
	require.NotContains(t, got.Extra, "unused_large_field")
}
