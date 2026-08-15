package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
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

func TestGatewayCandidatePoliciesKeepStableFirstRejectionAndInputOrder(t *testing.T) {
	service := &GatewayService{}
	accounts := []*Account{
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
		{ID: 2, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
		{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: false},
	}

	decision := service.evaluateGatewayCandidates(context.Background(), accounts, scheduling.Request{
		Platform:    PlatformAnthropic,
		ExcludedIDs: map[int64]struct{}{3: {}},
	})

	require.Equal(t, []*Account{accounts[0]}, decision.AllowedAccounts)
	require.Equal(t, []scheduling.CandidatePolicyTrace{
		{AccountID: 1, Allowed: true},
		{AccountID: 2, RejectedPolicy: "gateway_platform"},
		{AccountID: 3, RejectedPolicy: "not_excluded"},
	}, decision.Trace)
}

func TestGatewayCandidatePoliciesPutPrivacyBeforePlatformAndModel(t *testing.T) {
	service := &GatewayService{}
	group := &Group{RequirePrivacySet: true, Name: "private"}
	account := &Account{
		ID:          9,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
	}

	decision := scheduling.EvaluateCandidate(context.Background(), account, scheduling.Request{
		Platform: PlatformAnthropic,
	}, service.gatewayCandidatePoliciesFor(gatewayCandidatePolicyOptions{group: group})...)

	require.False(t, decision.Allowed)
	require.Equal(t, "privacy", decision.RejectedPolicy)
}
