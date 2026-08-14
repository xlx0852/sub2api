package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayLimitDecisionReportsFirstRejectedPolicy(t *testing.T) {
	service := &GatewayService{}
	account := &Account{
		ID:          7,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"quota_limit": 10.0,
			"quota_used":  10.0,
		},
	}

	decision := service.gatewayLimitDecision(context.Background(), account, false)

	require.False(t, decision.Allowed)
	require.Equal(t, "quota", decision.RejectedPolicy)
}

func TestGatewayLimitDecisionAllowsUnconfiguredLimits(t *testing.T) {
	service := &GatewayService{}
	account := &Account{
		ID:          7,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}

	decision := service.gatewayLimitDecision(context.Background(), account, false)

	require.Equal(t, true, decision.Allowed)
	require.Empty(t, decision.RejectedPolicy)
}

func TestGatewayPlatformPolicyKeepsMixedSchedulingSemantics(t *testing.T) {
	service := &GatewayService{}
	native := &Account{Platform: PlatformGemini}
	mixed := &Account{
		Platform: PlatformAntigravity,
		Extra:    map[string]any{"mixed_scheduling": true},
	}

	require.True(t, service.isAccountAllowedForPlatform(native, PlatformGemini, false))
	require.False(t, service.isAccountAllowedForPlatform(mixed, PlatformGemini, false))
	require.True(t, service.isAccountAllowedForPlatform(mixed, PlatformGemini, true))
}
