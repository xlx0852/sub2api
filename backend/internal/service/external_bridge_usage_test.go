package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestExternalBridgeAccountCredentialsAndURL(t *testing.T) {
	account := &domain.Account{
		Platform:    domain.PlatformOpenAI,
		Type:        domain.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "http://127.0.0.1:18401", "api_key": "bridge-token"},
		Extra:       map[string]any{"external_bridge": true, "external_bridge_kind": "cursor"},
	}
	require.True(t, account.IsExternalBridge())
	require.Equal(t, "cursor", account.ExternalBridgeKind())
	require.Equal(t, "http://127.0.0.1:18401", account.GetOpenAIBaseURL())
	require.Equal(t, "bridge-token", account.GetOpenAIApiKey())
	normalized, err := validateExternalBridgeBaseURL(account.GetOpenAIBaseURL())
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:18401", normalized)
}

func TestParseExternalBridgeUsage(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	usage := parseExternalBridgeUsage("devin", map[string]any{
		"plan_status": map[string]any{
			"available_prompt_credits": 12.5,
			"available_flow_credits":   3.0,
			"weekly_quota_remaining":   87.0,
			"weekly_quota_reset":       float64(reset),
			"plan_info":                map[string]any{"plan_name": "Pro"},
		},
	})
	require.Equal(t, "devin", usage.Kind)
	require.Equal(t, "Pro", usage.PlanName)
	require.Equal(t, 12.5, usage.AvailablePromptCredits)
	require.NotNil(t, usage.WeeklyQuotaRemaining)
	require.Equal(t, 87.0, *usage.WeeklyQuotaRemaining)
	require.NotNil(t, usage.WeeklyQuotaReset)

	cursor := parseExternalBridgeUsage("cursor", map[string]any{
		"sandUsageStatus": map[string]any{
			"usagePercent":          22.0,
			"hasAvailableUsage":     true,
			"nextResetTimestampUtc": "2030-01-01T00:00:00Z",
			"grokPlanLabel":         "SuperGrok",
		},
	})
	require.Equal(t, "SuperGrok", cursor.PlanName)
	require.NotNil(t, cursor.WeeklyQuotaRemaining)
	require.Equal(t, 78.0, *cursor.WeeklyQuotaRemaining)
	require.True(t, *cursor.CapacityAvailable)
}
