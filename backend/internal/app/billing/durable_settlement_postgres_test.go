package billing

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func durableSettlementPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("SUB2API_SETTLEMENT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SUB2API_SETTLEMENT_TEST_POSTGRES_DSN is not set")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(context.Background()))
	_, err = db.ExecContext(context.Background(), "TRUNCATE usage_settlement_attempts RESTART IDENTITY")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDurableSettlementPostgresRecoversExpiredProcessingLease(t *testing.T) {
	ctx := context.Background()
	db := durableSettlementPostgresDB(t)
	var userID, apiKeyID, accountID int64
	require.NoError(t, db.QueryRow(`
		SELECT u.id, k.id, a.id
		FROM users u
		JOIN api_keys k ON k.user_id = u.id AND k.name = 'settlement-openai'
		JOIN accounts a ON a.name = 'settlement-openai'
		WHERE u.email = 'settlement-admin@local.test'
		LIMIT 1
	`).Scan(&userID, &apiKeyID, &accountID))

	identity := "local:" + uuid.NewString()
	requestFingerprint := strings.Repeat("9", 64)
	command := &domain.UsageBillingCommand{
		RequestID: identity, APIKeyID: apiKeyID, UserID: userID, AccountID: accountID,
		AccountType: domain.AccountTypeAPIKey, Model: "gpt-4o-mini", InputTokens: 10,
		OutputTokens: 2, BalanceCost: 0.125, APIKeyQuotaCost: 0.125, AccountQuotaCost: 0.250,
	}
	command.Normalize()
	billingMode := "token"
	payload := domain.NewUsageSettlementPayloadV1(command, &domain.UsageLog{
		UserID: userID, APIKeyID: apiKeyID, AccountID: accountID, RequestID: identity,
		Model: "gpt-4o-mini", RequestedModel: "gpt-4o-mini", InputTokens: 10, OutputTokens: 2,
		TotalCost: 0.125, ActualCost: 0.125, RateMultiplier: 1, BillingType: domain.BillingTypeBalance,
		BillingMode: &billingMode, RequestType: domain.RequestTypeSync, CreatedAt: time.Now().UTC(),
	}, domain.PlatformOpenAI)

	attempts := repository.NewUsageSettlementRepository(db)
	created, inserted, err := attempts.CreateStarted(ctx, &domain.UsageSettlementAttempt{
		BillingIdentity: identity, APIKeyID: apiKeyID, RequestFingerprint: requestFingerprint,
		UserID: userID, AccountID: accountID, Provider: domain.PlatformOpenAI, RequestType: domain.RequestTypeSync,
		Authoritative: true,
	})
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = attempts.MarkReady(ctx, identity, apiKeyID, requestFingerprint, payload)
	require.NoError(t, err)
	claimed, err := attempts.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "crashed-instance", Limit: 1, LeaseDuration: time.Millisecond})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	time.Sleep(5 * time.Millisecond)

	worker := NewDurableSettlement(attempts, repository.NewUsageSettlementBillingRepository(db), DurableSettlementOptions{
		Owner: "restarted-instance", PollInterval: time.Hour, LeaseDuration: time.Second, BatchSize: 1,
	})
	worker.SetMode(SettlementDurabilityAuthoritative)
	worker.Start(ctx)
	worker.Wake()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := attempts.GetByIdentity(ctx, identity, apiKeyID)
		require.NoError(t, getErr)
		if current.State == domain.UsageSettlementStateSettled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	require.NoError(t, worker.StopAndWait(stopCtx))
	current, err := attempts.GetByIdentity(ctx, identity, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, created.ID, current.ID)
	require.Equal(t, domain.UsageSettlementStateSettled, current.State)
	var dedupCount, usageCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2`, identity, apiKeyID).Scan(&dedupCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2`, identity, apiKeyID).Scan(&usageCount))
	require.Equal(t, 1, dedupCount)
	require.Equal(t, 1, usageCount)
}
