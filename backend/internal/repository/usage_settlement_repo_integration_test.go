//go:build integration

package repository

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func settlementAttemptFixture(identity, fingerprint string, apiKeyID int64) *domain.UsageSettlementAttempt {
	deadline := time.Now().Add(time.Hour)
	return &domain.UsageSettlementAttempt{
		BillingIdentity: identity, APIKeyID: apiKeyID, RequestFingerprint: fingerprint,
		UserID: 10001, AccountID: 20001, Provider: domain.PlatformOpenAI,
		RequestType: domain.RequestTypeStream, InboundEndpoint: "/v1/responses",
		UpstreamEndpoint: "/v1/responses", ReconciliationDeadline: &deadline, Authoritative: true,
	}
}

func settlementPayloadFixture(identity string, apiKeyID int64, actualCost float64) *domain.UsageSettlementPayloadV1 {
	command := &domain.UsageBillingCommand{
		RequestID: identity, APIKeyID: apiKeyID, UserID: 10001, AccountID: 20001,
		AccountType: domain.AccountTypeAPIKey, Model: "gpt-fixture", InputTokens: 10,
		OutputTokens: 2, BalanceCost: actualCost, APIKeyQuotaCost: actualCost,
	}
	command.Normalize()
	return domain.NewUsageSettlementPayloadV1(command, &domain.UsageLog{
		UserID: 10001, APIKeyID: apiKeyID, AccountID: 20001, RequestID: identity,
		Model: "gpt-fixture", RequestedModel: "gpt-fixture", InputTokens: 10,
		OutputTokens: 2, ActualCost: actualCost, CreatedAt: time.Now().UTC(),
	}, domain.PlatformOpenAI)
}

func TestUsageSettlementRepositoryLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := NewUsageSettlementRepository(integrationDB)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("a", 64)

	created, inserted, err := repo.CreateStarted(ctx, settlementAttemptFixture(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, domain.UsageSettlementStateStarted, created.State)

	existing, inserted, err := repo.CreateStarted(ctx, settlementAttemptFixture(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, created.ID, existing.ID)

	_, _, err = repo.CreateStarted(ctx, settlementAttemptFixture(identity, strings.Repeat("b", 64), apiKeyID))
	require.ErrorIs(t, err, domain.ErrUsageSettlementFingerprintConflict)

	payload := settlementPayloadFixture(identity, apiKeyID, 0.25)
	ready, err := repo.MarkReady(ctx, identity, apiKeyID, fingerprint, payload)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, ready.State)
	require.NotEmpty(t, ready.PayloadHash)

	readyAgain, err := repo.MarkReady(ctx, identity, apiKeyID, fingerprint, payload)
	require.NoError(t, err)
	require.Equal(t, ready.PayloadHash, readyAgain.PayloadHash)
	_, err = repo.MarkReady(ctx, identity, apiKeyID, fingerprint, settlementPayloadFixture(identity, apiKeyID, 0.50))
	require.ErrorIs(t, err, domain.ErrUsageSettlementPayloadConflict)

	claimed, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "worker-a", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, created.ID, claimed[0].ID)
	require.Equal(t, "worker-a", claimed[0].LeaseOwner)

	claimedAgain, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "worker-b", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Empty(t, claimedAgain)
	require.ErrorIs(t, repo.MarkRetryable(ctx, created.ID, "worker-b", time.Now(), "TEMP", "temporary"), domain.ErrUsageSettlementLeaseConflict)
	require.NoError(t, repo.MarkRetryable(ctx, created.ID, "worker-a", time.Now().Add(-time.Second), "TEMP", "temporary"))

	claimed, err = repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "worker-b", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, 1, claimed[0].RetryCount)
	require.NoError(t, repo.MarkSettled(ctx, created.ID, "worker-b"))
	require.NoError(t, repo.MarkSettled(ctx, created.ID, "worker-b"), "commit-before-ack retry must be idempotent")

	settled, err := repo.GetByIdentity(ctx, identity, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateSettled, settled.State)
	require.NotNil(t, settled.SettledAt)
}

func TestUsageSettlementRepositoryExpiredLeaseHasSingleConcurrentWinner(t *testing.T) {
	ctx := context.Background()
	repo := NewUsageSettlementRepository(integrationDB)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("c", 64)
	created, _, err := repo.CreateStarted(ctx, settlementAttemptFixture(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	_, err = repo.MarkReady(ctx, identity, apiKeyID, fingerprint, settlementPayloadFixture(identity, apiKeyID, 0.25))
	require.NoError(t, err)
	first, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "expired-owner", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, first, 1)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_settlement_attempts SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`, created.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan []domain.UsageSettlementAttempt, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for _, owner := range []string{"worker-one", "worker-two"} {
		wait.Add(1)
		go func(owner string) {
			defer wait.Done()
			<-start
			claimed, claimErr := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: owner, Limit: 1, LeaseDuration: time.Minute})
			results <- claimed
			errorsCh <- claimErr
		}(owner)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for claimErr := range errorsCh {
		require.NoError(t, claimErr)
	}
	total := 0
	for claimed := range results {
		total += len(claimed)
	}
	require.Equal(t, 1, total)
}

func TestUsageSettlementRepositoryReconciliationCanRecoverToReady(t *testing.T) {
	ctx := context.Background()
	repo := NewUsageSettlementRepository(integrationDB)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("d", 64)
	_, _, err := repo.CreateStarted(ctx, settlementAttemptFixture(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	require.NoError(t, repo.MarkReconciliationRequired(ctx, identity, apiKeyID, fingerprint, "USAGE_MISSING", "terminal usage unavailable"))
	reconciled, err := repo.GetByIdentity(ctx, identity, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReconciliationRequired, reconciled.State)
	ready, err := repo.MarkReady(ctx, identity, apiKeyID, fingerprint, settlementPayloadFixture(identity, apiKeyID, 0.25))
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, ready.State)
}
