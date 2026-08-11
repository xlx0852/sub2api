package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuantizeUsageBillingAmountCeilsPositiveChargesToSixPlaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "zero", in: 0, want: 0},
		{name: "smallest positive", in: 0.000000001, want: 0.000001},
		{name: "already exact", in: 0.123456, want: 0.123456},
		{name: "one digit beyond scale", in: 0.123456001, want: 0.123457},
		{name: "binary tail on exact amount", in: 0.036000000000000004, want: 0.036},
		{name: "meaningful tenth decimal tail", in: 0.1234560001, want: 0.123457},
		{name: "long tail", in: 0.0000781234567, want: 0.000079},
		{name: "large value", in: 999999.123456001, want: 999999.123457},
		{name: "negative unchanged", in: -0.123456001, want: -0.123456001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, QuantizeUsageBillingAmount(tt.in))
		})
	}

	require.True(t, math.IsNaN(QuantizeUsageBillingAmount(math.NaN())))
	require.True(t, math.IsInf(QuantizeUsageBillingAmount(math.Inf(1)), 1))
	require.True(t, math.IsInf(QuantizeUsageBillingAmount(math.Inf(-1)), -1))
}

func TestUsageBillingCommandNormalizesEveryMonetaryEffectAfterFingerprint(t *testing.T) {
	t.Parallel()

	cmd := &UsageBillingCommand{
		RequestID:           "req-six-decimal-ceiling",
		UserID:              1,
		APIKeyID:            2,
		AccountID:           3,
		BalanceCost:         0.0000781234567,
		SubscriptionCost:    0.123456001,
		APIKeyQuotaCost:     0.000000001,
		APIKeyRateLimitCost: 1.000000001,
		AccountQuotaCost:    0.333333333,
	}
	wantFingerprint := buildUsageBillingFingerprint(cmd)

	cmd.Normalize()

	require.Equal(t, wantFingerprint, cmd.RequestFingerprint)
	require.Equal(t, 0.000079, cmd.BalanceCost)
	require.Equal(t, 0.123457, cmd.SubscriptionCost)
	require.Equal(t, 0.000001, cmd.APIKeyQuotaCost)
	require.Equal(t, 1.000001, cmd.APIKeyRateLimitCost)
	require.Equal(t, 0.333334, cmd.AccountQuotaCost)
}

func TestApplyCanonicalUsageBillingAmountsKeepsCustomerLedgersAligned(t *testing.T) {
	t.Parallel()

	usageLog := &UsageLog{ActualCost: 0.0000781234567}
	params := &postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 0.0000781234567}}
	cmd := &UsageBillingCommand{
		BalanceCost:         0.0000781234567,
		APIKeyQuotaCost:     0.0000781234567,
		APIKeyRateLimitCost: 0.0000781234567,
		AccountQuotaCost:    0.0000625001,
	}
	cmd.Normalize()

	applyCanonicalUsageBillingAmounts(usageLog, params, cmd)

	require.Equal(t, 0.000079, cmd.BalanceCost)
	require.Equal(t, cmd.BalanceCost, cmd.APIKeyQuotaCost)
	require.Equal(t, cmd.BalanceCost, cmd.APIKeyRateLimitCost)
	require.Equal(t, cmd.BalanceCost, params.Cost.ActualCost)
	require.Equal(t, cmd.BalanceCost, usageLog.ActualCost)
	require.Equal(t, 0.000063, cmd.AccountQuotaCost,
		"account-side quota keeps its own cost basis while sharing the precision contract")
}

func TestApplyCanonicalUsageBillingAmountsUsesSubscriptionCharge(t *testing.T) {
	t.Parallel()

	usageLog := &UsageLog{ActualCost: 0.123456001}
	params := &postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 0.123456001}}
	cmd := &UsageBillingCommand{SubscriptionCost: 0.123456001}
	cmd.Normalize()

	applyCanonicalUsageBillingAmounts(usageLog, params, cmd)

	require.Equal(t, 0.123457, params.Cost.ActualCost)
	require.Equal(t, params.Cost.ActualCost, usageLog.ActualCost)
}
