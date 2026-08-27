package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageSettlementAttemptCanRebindStartedAccount(t *testing.T) {
	started := &UsageSettlementAttempt{
		BillingIdentity: "client:failover", RequestFingerprint: strings.Repeat("a", 64),
		APIKeyID: 84, UserID: 37, AccountID: 108, State: UsageSettlementStateStarted,
	}
	require.True(t, started.CanRebindStartedAccount(37, 73, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(37, 108, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(38, 73, started.RequestFingerprint))
	require.False(t, started.CanRebindStartedAccount(37, 73, strings.Repeat("b", 64)))

	started.PayloadHash = strings.Repeat("c", 64)
	require.False(t, started.CanRebindStartedAccount(37, 73, started.RequestFingerprint))

	ready := &UsageSettlementAttempt{
		BillingIdentity: "client:failover", RequestFingerprint: strings.Repeat("a", 64),
		APIKeyID: 84, UserID: 37, AccountID: 108, State: UsageSettlementStateReady,
	}
	require.False(t, ready.CanRebindStartedAccount(37, 73, ready.RequestFingerprint))
}

func TestUsageSettlementAttemptRejectsOversizedRequestFingerprint(t *testing.T) {
	attempt := &UsageSettlementAttempt{
		BillingIdentity: "local:test", RequestFingerprint: strings.Repeat("f", 65),
		APIKeyID: 1, UserID: 1, AccountID: 1, Provider: "openai",
		State: UsageSettlementStateStarted,
	}
	require.EqualError(t, attempt.ValidateStarted(), "usage settlement request fingerprint exceeds 64 characters")
}

func TestUsageSettlementTransitions(t *testing.T) {
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateStarted, UsageSettlementStateReady))
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateProcessing, UsageSettlementStateSettled))
	require.True(t, CanTransitionUsageSettlement(UsageSettlementStateReconciliationRequired, UsageSettlementStateReady))
	require.False(t, CanTransitionUsageSettlement(UsageSettlementStateSettled, UsageSettlementStateReady))
	require.ErrorIs(t, ValidateUsageSettlementTransition(UsageSettlementStateSettled, UsageSettlementStateReady), ErrUsageSettlementInvalidTransition)
}

func TestUsageSettlementPayloadRoundTripExcludesRelations(t *testing.T) {
	createdAt := time.Unix(1700000000, 0).UTC()
	pricingCurrency := string(PricingCurrencyCNY)
	preUpstreamMs, totalMs := 120, 980
	command := &UsageBillingCommand{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4, AccountType: AccountTypeAPIKey, InputTokens: 10, BalanceCost: 0.1}
	command.Normalize()
	log := &UsageLog{
		UserID: 3, APIKeyID: 2, AccountID: 4, RequestID: "local:turn-1", Model: "gpt-test",
		InputTokens: 10, ActualCost: 0.1, PricingCurrency: &pricingCurrency, CreatedAt: createdAt,
		FirstOutputStages: &FirstOutputStages{GatewayPreUpstreamMs: &preUpstreamMs, GatewayFirstOutputTotalMs: &totalMs},
		User:              &User{Email: "must-not-be-serialized@example.com"},
		Account:           &Account{Credentials: map[string]any{"access_token": "secret"}},
	}
	payload := NewUsageSettlementPayloadV1(command, log, "openai")
	encoded, hash, err := payload.CanonicalJSON("local:turn-1", 2, 3, 4)
	require.NoError(t, err)
	require.Len(t, hash, 64)
	require.NotContains(t, string(encoded), "must-not-be-serialized")
	require.NotContains(t, string(encoded), "access_token")

	var decoded UsageSettlementPayloadV1
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, command.RequestFingerprint, decoded.Billing.RequestFingerprint)
	require.Equal(t, createdAt, decoded.UsageLog.CreatedAt)
	require.Equal(t, log.RequestID, decoded.UsageLog.UsageLog().RequestID)
	require.Equal(t, pricingCurrency, *decoded.UsageLog.UsageLog().PricingCurrency)
	require.Equal(t, preUpstreamMs, *decoded.UsageLog.UsageLog().FirstOutputStages.GatewayPreUpstreamMs)
	require.Equal(t, totalMs, *decoded.UsageLog.UsageLog().FirstOutputStages.GatewayFirstOutputTotalMs)
}

func TestUsageSettlementPayloadRejectsIdentityMismatch(t *testing.T) {
	command := &UsageBillingCommand{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4}
	command.Normalize()
	payload := NewUsageSettlementPayloadV1(command, &UsageLog{RequestID: "local:turn-1", APIKeyID: 2, UserID: 3, AccountID: 4}, "")
	require.Error(t, payload.Validate("local:turn-2", 2, 3, 4))
}

func TestValidateUsageSettlementPayloadJSONRejectsPromptResponseAndCredentials(t *testing.T) {
	for name, encoded := range map[string][]byte{
		"prompt":      []byte(`{"usage_log":{"prompt":"secret"}}`),
		"messages":    []byte(`{"usage_log":{"messages":[]}}`),
		"response":    []byte(`{"usage_log":{"response_body":"secret"}}`),
		"credentials": []byte(`{"billing":{"credentials":{"access_token":"secret"}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, ValidateUsageSettlementPayloadJSON(encoded), ErrUsageSettlementSensitivePayload)
		})
	}
	require.NoError(t, ValidateUsageSettlementPayloadJSON([]byte(`{"usage_log":{"input_tokens":10,"output_tokens":2,"user_agent":"codex/test"}}`)))
}
