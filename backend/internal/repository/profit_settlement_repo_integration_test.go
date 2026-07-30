//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProfitRepositorySubscriptionBanSettlementDisablesAccountAndShadowAtomically(t *testing.T) {
	ctx := context.Background()
	parent := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "ban-settlement-parent", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true,
	})
	parentID := parent.ID
	shadow := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "ban-settlement-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true, ParentAccountID: &parentID, QuotaDimension: service.QuotaDimensionSpark,
	})
	start := time.Now().UTC().AddDate(0, 0, -5).Truncate(24 * time.Hour)
	var cycleID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO account_subscription_cycles (account_id, starts_at, period_fee, period_days, currency, notes)
		VALUES ($1, $2, 865, 30, 'USD', 'integration')
		RETURNING id
	`, parent.ID, start).Scan(&cycleID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM scheduler_outbox WHERE account_id IN ($1, $2)`, parent.ID, shadow.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_subscription_refunds WHERE termination_id IN (SELECT id FROM account_subscription_terminations WHERE cycle_id = $1)`, cycleID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_subscription_terminations WHERE cycle_id = $1`, cycleID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_subscription_cycles WHERE id = $1`, cycleID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, shadow.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, parent.ID)
	})

	repo := &profitRepository{db: integrationDB}
	bannedAt := start.AddDate(0, 0, 5)
	result, err := repo.CreateSubscriptionTermination(ctx, &service.AccountSubscriptionTermination{
		CycleID: cycleID, EffectiveAt: bannedAt, Reason: "upstream_banned", Notes: "integration",
	}, &service.AccountSubscriptionRefund{Amount: 200, ReceivedAt: bannedAt, Notes: "partial"})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{parent.ID, shadow.ID}, result.DisabledAccountIDs)

	for _, accountID := range result.DisabledAccountIDs {
		var status string
		var schedulable bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, schedulable FROM accounts WHERE id = $1`, accountID).Scan(&status, &schedulable))
		require.Equal(t, service.StatusDisabled, status)
		require.False(t, schedulable)
	}
	cycles, err := repo.ListSubscriptionCycles(ctx, parent.ID)
	require.NoError(t, err)
	require.Len(t, cycles, 1)
	require.NotNil(t, cycles[0].Termination)
	require.Len(t, cycles[0].Refunds, 1)
	require.Equal(t, 200.0, cycles[0].Refunds[0].Amount)

	_, err = repo.CreateSubscriptionRefund(ctx, &service.AccountSubscriptionRefund{
		TerminationID: result.Termination.ID, Amount: 700, ReceivedAt: bannedAt.Add(time.Minute),
	})
	require.ErrorIs(t, err, service.ErrSubscriptionRefundExceedsFee)

	voided, err := repo.VoidSubscriptionRefund(ctx, result.InitialRefund.ID, "wrong amount", bannedAt.Add(2*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, voided.VoidedAt)
	added, err := repo.CreateSubscriptionRefund(ctx, &service.AccountSubscriptionRefund{
		TerminationID: result.Termination.ID, Amount: 100, ReceivedAt: bannedAt.Add(3 * time.Minute), Notes: "corrected",
	})
	require.NoError(t, err)
	require.Equal(t, 100.0, added.Amount)
	reversed, err := repo.ReverseSubscriptionTermination(ctx, result.Termination.ID, "provider restored", bannedAt.Add(4*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, reversed.ReversedAt)

	var parentStatus string
	var parentSchedulable bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status, schedulable FROM accounts WHERE id = $1`, parent.ID).Scan(&parentStatus, &parentSchedulable))
	require.Equal(t, service.StatusDisabled, parentStatus)
	require.False(t, parentSchedulable, "financial reversal must not reactivate scheduling")
	require.ErrorIs(t, repo.DeleteSubscriptionCycle(ctx, cycleID), service.ErrSubscriptionCycleSettled)
}
