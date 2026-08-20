package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func settlementPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("SUB2API_SETTLEMENT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SUB2API_SETTLEMENT_TEST_POSTGRES_DSN is not set")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname(), "destructive settlement test DSN must be loopback")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(context.Background()))
	require.NoError(t, ApplyMigrations(context.Background(), db))
	_, err = db.ExecContext(context.Background(), "TRUNCATE usage_settlement_attempts RESTART IDENTITY")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func postgresSettlementAttempt(identity, fingerprint string, apiKeyID int64) *domain.UsageSettlementAttempt {
	deadline := time.Now().Add(time.Hour)
	return &domain.UsageSettlementAttempt{
		BillingIdentity: identity, APIKeyID: apiKeyID, RequestFingerprint: fingerprint,
		UserID: 31001, AccountID: 32001, Provider: domain.PlatformOpenAI,
		RequestType: domain.RequestTypeStream, InboundEndpoint: "/v1/responses",
		UpstreamEndpoint: "/v1/responses", ReconciliationDeadline: &deadline, Authoritative: true,
	}
}

func TestUsageSettlementRepositoryPostgresNeverClaimsShadowAttempts(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	shadowIdentity := "local:" + uuid.NewString()
	authoritativeIdentity := "local:" + uuid.NewString()
	shadow := postgresSettlementAttempt(shadowIdentity, strings.Repeat("7", 64), time.Now().UnixNano())
	shadow.Authoritative = false
	authoritative := postgresSettlementAttempt(authoritativeIdentity, strings.Repeat("8", 64), time.Now().UnixNano()+1)

	shadowCreated, inserted, err := repo.CreateStarted(ctx, shadow)
	require.NoError(t, err)
	require.True(t, inserted)
	require.False(t, shadowCreated.Authoritative)
	_, err = repo.MarkReady(ctx, shadowIdentity, shadow.APIKeyID, shadow.RequestFingerprint, postgresSettlementPayload(shadowIdentity, shadow.APIKeyID, 0.25))
	require.NoError(t, err)

	// A later authoritative retry of the same identity must not upgrade a shadow row.
	shadow.Authoritative = true
	existing, inserted, err := repo.CreateStarted(ctx, shadow)
	require.NoError(t, err)
	require.False(t, inserted)
	require.False(t, existing.Authoritative)

	authoritativeCreated, inserted, err := repo.CreateStarted(ctx, authoritative)
	require.NoError(t, err)
	require.True(t, inserted)
	require.True(t, authoritativeCreated.Authoritative)
	_, err = repo.MarkReady(ctx, authoritativeIdentity, authoritative.APIKeyID, authoritative.RequestFingerprint, postgresSettlementPayload(authoritativeIdentity, authoritative.APIKeyID, 0.25))
	require.NoError(t, err)

	claimed, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "authoritative-worker", Limit: 10, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, authoritativeCreated.ID, claimed[0].ID)
	shadowAfter, err := repo.GetByIdentity(ctx, shadowIdentity, shadow.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, shadowAfter.State)
}

func postgresSettlementPayload(identity string, apiKeyID int64, actualCost float64) *domain.UsageSettlementPayloadV1 {
	command := &domain.UsageBillingCommand{
		RequestID: identity, APIKeyID: apiKeyID, UserID: 31001, AccountID: 32001,
		AccountType: domain.AccountTypeAPIKey, Model: "gpt-fixture", InputTokens: 10,
		OutputTokens: 2, BalanceCost: actualCost, APIKeyQuotaCost: actualCost,
	}
	command.Normalize()
	return domain.NewUsageSettlementPayloadV1(command, &domain.UsageLog{
		UserID: 31001, APIKeyID: apiKeyID, AccountID: 32001, RequestID: identity,
		Model: "gpt-fixture", RequestedModel: "gpt-fixture", InputTokens: 10,
		OutputTokens: 2, ActualCost: actualCost, CreatedAt: time.Now().UTC(),
	}, domain.PlatformOpenAI)
}

func TestUsageSettlementRepositoryPostgresStateMachine(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("e", 64)

	initial := postgresSettlementAttempt(identity, fingerprint, apiKeyID)
	initial.RecoveryContext = &domain.UsageSettlementRecoveryContextV1{
		Version: 1, RequestPayloadHash: strings.Repeat("1", 64), OriginalModel: "gpt-5.1",
		MappedModel: "gpt-5.1", UserAgent: "settlement-test",
	}
	created, inserted, err := repo.CreateStarted(ctx, initial)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NotNil(t, created.RecoveryContext)
	require.Equal(t, "gpt-5.1", created.RecoveryContext.OriginalModel)
	_, inserted, err = repo.CreateStarted(ctx, postgresSettlementAttempt(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	require.False(t, inserted)
	observed, err := repo.ObserveProviderRequestID(ctx, identity, apiKeyID, fingerprint, "resp_stored_1")
	require.NoError(t, err)
	require.Equal(t, "resp_stored_1", observed.ProviderRequestID)
	observed, err = repo.ObserveProviderRequestID(ctx, identity, apiKeyID, fingerprint, "resp_stored_1")
	require.NoError(t, err)
	require.Equal(t, "resp_stored_1", observed.ProviderRequestID)
	_, err = repo.ObserveProviderRequestID(ctx, identity, apiKeyID, fingerprint, "resp_stored_other")
	require.ErrorIs(t, err, domain.ErrUsageSettlementProviderIDConflict)
	_, _, err = repo.CreateStarted(ctx, postgresSettlementAttempt(identity, strings.Repeat("f", 64), apiKeyID))
	require.ErrorIs(t, err, domain.ErrUsageSettlementFingerprintConflict)

	payload := postgresSettlementPayload(identity, apiKeyID, 0.25)
	ready, err := repo.MarkReady(ctx, identity, apiKeyID, fingerprint, payload)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, ready.State)
	_, err = repo.MarkReady(ctx, identity, apiKeyID, fingerprint, postgresSettlementPayload(identity, apiKeyID, 0.50))
	require.ErrorIs(t, err, domain.ErrUsageSettlementPayloadConflict)

	claimed, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "postgres-worker-a", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, created.ID, claimed[0].ID)
	require.ErrorIs(t, repo.MarkRetryable(ctx, created.ID, "wrong-owner", time.Now(), "TEMP", "temporary"), domain.ErrUsageSettlementLeaseConflict)
	require.NoError(t, repo.MarkRetryable(ctx, created.ID, "postgres-worker-a", time.Now().Add(-time.Second), "TEMP", "temporary"))
	claimed, err = repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "postgres-worker-b", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NoError(t, repo.MarkSettled(ctx, created.ID, "postgres-worker-b"))
	require.NoError(t, repo.MarkSettled(ctx, created.ID, "postgres-worker-b"))

	settled, err := repo.GetByIdentity(ctx, identity, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateSettled, settled.State)
	require.NotNil(t, settled.SettledAt)
}

func TestUsageSettlementRepositoryPostgresExpiredLeaseSingleWinner(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("a", 64)
	created, _, err := repo.CreateStarted(ctx, postgresSettlementAttempt(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	_, err = repo.MarkReady(ctx, identity, apiKeyID, fingerprint, postgresSettlementPayload(identity, apiKeyID, 0.25))
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: "expired-owner", Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	_, err = db.ExecContext(ctx, `UPDATE usage_settlement_attempts SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`, created.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan int, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for _, owner := range []string{"postgres-worker-one", "postgres-worker-two"} {
		wait.Add(1)
		go func(owner string) {
			defer wait.Done()
			<-start
			items, claimErr := repo.Claim(ctx, ports.UsageSettlementClaimRequest{Owner: owner, Limit: 1, LeaseDuration: time.Minute})
			results <- len(items)
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
	for count := range results {
		total += count
	}
	require.Equal(t, 1, total)
}

func TestUsageSettlementRepositoryPostgresClaimFiltersRolloutGroups(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	selectedGroup, otherGroup := int64(9102), int64(9101)
	type fixture struct {
		identity string
		apiKeyID int64
		groupID  *int64
	}
	fixtures := []fixture{
		{identity: "local:" + uuid.NewString(), apiKeyID: time.Now().UnixNano(), groupID: &otherGroup},
		{identity: "local:" + uuid.NewString(), apiKeyID: time.Now().UnixNano() + 1, groupID: &selectedGroup},
	}
	for _, item := range fixtures {
		attempt := postgresSettlementAttempt(item.identity, strings.Repeat("a", 64), item.apiKeyID)
		attempt.GroupID = item.groupID
		_, inserted, err := repo.CreateStarted(ctx, attempt)
		require.NoError(t, err)
		require.True(t, inserted)
		_, err = repo.MarkReady(ctx, item.identity, item.apiKeyID, attempt.RequestFingerprint, postgresSettlementPayload(item.identity, item.apiKeyID, 0.25))
		require.NoError(t, err)
	}

	claimed, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{
		Owner: "scoped-worker", Limit: 10, LeaseDuration: time.Minute, GroupIDs: []int64{selectedGroup},
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NotNil(t, claimed[0].GroupID)
	require.Equal(t, selectedGroup, *claimed[0].GroupID)

	remaining, err := repo.Claim(ctx, ports.UsageSettlementClaimRequest{
		Owner: "global-worker", Limit: 10, LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, otherGroup, *remaining[0].GroupID)
}

func TestUsageSettlementRepositoryPostgresReconciliationRecovery(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	identity := "local:" + uuid.NewString()
	apiKeyID := time.Now().UnixNano()
	fingerprint := strings.Repeat("b", 64)
	_, _, err := repo.CreateStarted(ctx, postgresSettlementAttempt(identity, fingerprint, apiKeyID))
	require.NoError(t, err)
	require.NoError(t, repo.MarkReconciliationRequired(ctx, identity, apiKeyID, fingerprint, "USAGE_MISSING", "terminal usage unavailable"))
	ready, err := repo.MarkReady(ctx, identity, apiKeyID, fingerprint, postgresSettlementPayload(identity, apiKeyID, 0.25))
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, ready.State)
}

func TestUsageSettlementRepositoryPostgresExpiresOnlyOverdueStartedAttempts(t *testing.T) {
	ctx := context.Background()
	db := settlementPostgresDB(t)
	repo := NewUsageSettlementRepository(db)
	now := time.Now().UTC()
	overdue := postgresSettlementAttempt("local:"+uuid.NewString(), strings.Repeat("c", 64), time.Now().UnixNano())
	overdueDeadline := now.Add(-time.Minute)
	overdue.ReconciliationDeadline = &overdueDeadline
	overdue.RecoveryContext = &domain.UsageSettlementRecoveryContextV1{
		Version: 1, RequestPayloadHash: strings.Repeat("2", 64), OriginalModel: "gpt-5.1",
	}
	future := postgresSettlementAttempt("local:"+uuid.NewString(), strings.Repeat("d", 64), time.Now().UnixNano()+1)
	futureDeadline := now.Add(time.Hour)
	future.ReconciliationDeadline = &futureDeadline
	ready := postgresSettlementAttempt("local:"+uuid.NewString(), strings.Repeat("e", 64), time.Now().UnixNano()+2)
	readyDeadline := now.Add(-time.Minute)
	ready.ReconciliationDeadline = &readyDeadline

	_, _, err := repo.CreateStarted(ctx, overdue)
	require.NoError(t, err)
	_, err = repo.ObserveProviderRequestID(ctx, overdue.BillingIdentity, overdue.APIKeyID, overdue.RequestFingerprint, "resp_overdue")
	require.NoError(t, err)
	_, _, err = repo.CreateStarted(ctx, future)
	require.NoError(t, err)
	_, _, err = repo.CreateStarted(ctx, ready)
	require.NoError(t, err)
	_, err = repo.MarkReady(ctx, ready.BillingIdentity, ready.APIKeyID, ready.RequestFingerprint, postgresSettlementPayload(ready.BillingIdentity, ready.APIKeyID, 0.25))
	require.NoError(t, err)
	recoverable, err := repo.ListExpiredRecoverable(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	require.Equal(t, "resp_overdue", recoverable[0].ProviderRequestID)

	expired, err := repo.ExpireStarted(ctx, now, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, expired)
	overdueAfter, err := repo.GetByIdentity(ctx, overdue.BillingIdentity, overdue.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReconciliationRequired, overdueAfter.State)
	require.Equal(t, "TERMINAL_USAGE_MISSING", overdueAfter.LastErrorCode)
	require.Nil(t, overdueAfter.Payload)
	futureAfter, err := repo.GetByIdentity(ctx, future.BillingIdentity, future.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateStarted, futureAfter.State)
	readyAfter, err := repo.GetByIdentity(ctx, ready.BillingIdentity, ready.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateReady, readyAfter.State)
	stats, err := repo.BacklogStats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.Started)
	require.EqualValues(t, 1, stats.Ready)
	require.EqualValues(t, 0, stats.Processing)
	require.EqualValues(t, 0, stats.Retryable)
	require.EqualValues(t, 1, stats.ReconciliationRequired)
	require.NotNil(t, stats.OldestPendingAt)
}

type settlementBillingState struct {
	Balance      float64
	APIKeyQuota  float64
	AccountQuota float64
	DedupCount   int
	UsageCount   int
}

func settlementHarnessSubjects(t *testing.T, db *sql.DB) (userID, apiKeyID, accountID int64) {
	t.Helper()
	err := db.QueryRow(`
		SELECT u.id, k.id, a.id
		FROM users u
		JOIN api_keys k ON k.user_id = u.id AND k.name = 'settlement-openai'
		JOIN accounts a ON a.name = 'settlement-openai'
		WHERE u.email = 'settlement-admin@local.test'
		LIMIT 1
	`).Scan(&userID, &apiKeyID, &accountID)
	if err == nil {
		return userID, apiKeyID, accountID
	}
	require.ErrorIs(t, err, sql.ErrNoRows)

	require.NoError(t, db.QueryRow(`
		INSERT INTO users (email, password_hash, role, status, balance, concurrency)
		VALUES ('settlement-admin@local.test', 'hash', 'admin', 'active', 100, 5)
		RETURNING id
	`).Scan(&userID))
	require.NoError(t, db.QueryRow(`
		INSERT INTO api_keys (user_id, key, name, status, quota, quota_used)
		VALUES ($1, 'sk-settlement-openai-test', 'settlement-openai', 'active', 100, 0)
		RETURNING id
	`, userID).Scan(&apiKeyID))
	require.NoError(t, db.QueryRow(`
		INSERT INTO accounts (name, platform, type, status, extra)
		VALUES ('settlement-openai', 'openai', 'apikey', 'active', '{"quota_used": 0}'::jsonb)
		RETURNING id
	`).Scan(&accountID))
	return userID, apiKeyID, accountID
}

func readSettlementBillingState(t *testing.T, db *sql.DB, identity string, userID, apiKeyID, accountID int64) settlementBillingState {
	t.Helper()
	var state settlementBillingState
	require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id = $1`, userID).Scan(&state.Balance))
	require.NoError(t, db.QueryRow(`SELECT quota_used FROM api_keys WHERE id = $1`, apiKeyID).Scan(&state.APIKeyQuota))
	require.NoError(t, db.QueryRow(`SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1`, accountID).Scan(&state.AccountQuota))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2`, identity, apiKeyID).Scan(&state.DedupCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2`, identity, apiKeyID).Scan(&state.UsageCount))
	return state
}

func atomicSettlementPayload(identity string, userID, apiKeyID, accountID int64, invalidUsageLog bool) *domain.UsageSettlementPayloadV1 {
	command := &domain.UsageBillingCommand{
		RequestID: identity, APIKeyID: apiKeyID, UserID: userID, AccountID: accountID,
		AccountType: domain.AccountTypeAPIKey, Model: "gpt-4o-mini", InputTokens: 10, OutputTokens: 2,
		BalanceCost: 0.25, APIKeyQuotaCost: 0.25, AccountQuotaCost: 0.50,
	}
	command.Normalize()
	billingMode := "token"
	log := &domain.UsageLog{
		UserID: userID, APIKeyID: apiKeyID, AccountID: accountID, RequestID: identity,
		Model: "gpt-4o-mini", RequestedModel: "gpt-4o-mini", InputTokens: 10, OutputTokens: 2,
		InputCost: 0.20, OutputCost: 0.05, TotalCost: 0.25, ActualCost: 0.25,
		RateMultiplier: 1, BillingType: domain.BillingTypeBalance, RequestType: domain.RequestTypeSync,
		BillingMode: &billingMode, CreatedAt: time.Now().UTC(),
	}
	if invalidUsageLog {
		invalidMode := "image"
		log.BillingMode = &invalidMode
		log.ImageCount = 1
	}
	return domain.NewUsageSettlementPayloadV1(command, log, domain.PlatformOpenAI)
}

func createClaimedAtomicSettlement(t *testing.T, db *sql.DB, identity, requestFingerprint, owner string, userID, apiKeyID, accountID int64, invalidUsageLog bool) *domain.UsageSettlementAttempt {
	t.Helper()
	settlements := NewUsageSettlementRepository(db)
	attempt := &domain.UsageSettlementAttempt{
		BillingIdentity: identity, APIKeyID: apiKeyID, RequestFingerprint: requestFingerprint,
		UserID: userID, AccountID: accountID, Provider: domain.PlatformOpenAI, RequestType: domain.RequestTypeSync,
		Authoritative: true,
	}
	created, inserted, err := settlements.CreateStarted(context.Background(), attempt)
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = settlements.MarkReady(context.Background(), identity, apiKeyID, requestFingerprint, atomicSettlementPayload(identity, userID, apiKeyID, accountID, invalidUsageLog))
	require.NoError(t, err)
	claimed, err := settlements.Claim(context.Background(), ports.UsageSettlementClaimRequest{Owner: owner, Limit: 1, LeaseDuration: time.Minute})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, created.ID, claimed[0].ID)
	return &claimed[0]
}

func TestUsageSettlementBillingPostgresCommitsEffectsLogAndStateAtomically(t *testing.T) {
	db := settlementPostgresDB(t)
	userID, apiKeyID, accountID := settlementHarnessSubjects(t, db)
	identity := "local:" + uuid.NewString()
	owner := "atomic-worker"
	attempt := createClaimedAtomicSettlement(t, db, identity, strings.Repeat("c", 64), owner, userID, apiKeyID, accountID, false)
	before := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)

	billing := NewUsageSettlementBillingRepository(db)
	result, err := billing.ApplySettlement(context.Background(), attempt.ID, owner)
	require.NoError(t, err)
	require.True(t, result.Billing.Applied)
	require.True(t, result.UsageLogInserted)
	after := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	require.InDelta(t, before.Balance-0.25, after.Balance, 0.000001)
	require.InDelta(t, before.APIKeyQuota+0.25, after.APIKeyQuota, 0.000001)
	require.InDelta(t, before.AccountQuota+0.50, after.AccountQuota, 0.000001)
	require.Equal(t, 1, after.DedupCount)
	require.Equal(t, 1, after.UsageCount)

	settled, err := NewUsageSettlementRepository(db).GetByIdentity(context.Background(), identity, apiKeyID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageSettlementStateSettled, settled.State)
	result, err = billing.ApplySettlement(context.Background(), attempt.ID, owner)
	require.NoError(t, err)
	require.True(t, result.AlreadySettled)
	require.Equal(t, after, readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID))
}

func TestUsageSettlementBillingPostgresUsageLogFailureRollsBackEverything(t *testing.T) {
	db := settlementPostgresDB(t)
	userID, apiKeyID, accountID := settlementHarnessSubjects(t, db)
	identity := "local:" + uuid.NewString()
	owner := "rollback-worker"
	attempt := createClaimedAtomicSettlement(t, db, identity, strings.Repeat("d", 64), owner, userID, apiKeyID, accountID, true)
	before := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)

	_, err := NewUsageSettlementBillingRepository(db).ApplySettlement(context.Background(), attempt.ID, owner)
	require.Error(t, err)
	after := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	require.Equal(t, before, after)
	current, getErr := NewUsageSettlementRepository(db).GetByIdentity(context.Background(), identity, apiKeyID)
	require.NoError(t, getErr)
	require.Equal(t, domain.UsageSettlementStateProcessing, current.State)
}

func TestUsageSettlementBillingPostgresConcurrentApplyChargesOnce(t *testing.T) {
	db := settlementPostgresDB(t)
	userID, apiKeyID, accountID := settlementHarnessSubjects(t, db)
	identity := "local:" + uuid.NewString()
	owner := "concurrent-atomic-worker"
	attempt := createClaimedAtomicSettlement(t, db, identity, strings.Repeat("e", 64), owner, userID, apiKeyID, accountID, false)
	before := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	billing := NewUsageSettlementBillingRepository(db)

	start := make(chan struct{})
	results := make(chan *domain.UsageSettlementApplyResult, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := billing.ApplySettlement(context.Background(), attempt.ID, owner)
			results <- result
			errorsCh <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}
	applied, already := 0, 0
	for result := range results {
		if result.Billing.Applied {
			applied++
		}
		if result.AlreadySettled {
			already++
		}
	}
	require.Equal(t, 1, applied)
	require.Equal(t, 1, already)
	after := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	require.InDelta(t, before.Balance-0.25, after.Balance, 0.000001)
	require.Equal(t, 1, after.DedupCount)
	require.Equal(t, 1, after.UsageCount)
}

func TestUsageSettlementBillingPostgresDedupFingerprintConflictHasNoEffects(t *testing.T) {
	db := settlementPostgresDB(t)
	userID, apiKeyID, accountID := settlementHarnessSubjects(t, db)
	identity := "local:" + uuid.NewString()
	owner := "fingerprint-conflict-worker"
	attempt := createClaimedAtomicSettlement(t, db, identity, strings.Repeat("f", 64), owner, userID, apiKeyID, accountID, false)
	before := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	_, err := db.Exec(`
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES ($1, $2, $3)
	`, identity, apiKeyID, strings.Repeat("0", 64))
	require.NoError(t, err)

	_, err = NewUsageSettlementBillingRepository(db).ApplySettlement(context.Background(), attempt.ID, owner)
	require.ErrorIs(t, err, domain.ErrUsageBillingRequestConflict)
	after := readSettlementBillingState(t, db, identity, userID, apiKeyID, accountID)
	require.InDelta(t, before.Balance, after.Balance, 0.000001)
	require.InDelta(t, before.APIKeyQuota, after.APIKeyQuota, 0.000001)
	require.InDelta(t, before.AccountQuota, after.AccountQuota, 0.000001)
	require.Equal(t, 1, after.DedupCount, "preexisting conflicting dedup row remains the only row")
	require.Equal(t, 0, after.UsageCount)
	current, getErr := NewUsageSettlementRepository(db).GetByIdentity(context.Background(), identity, apiKeyID)
	require.NoError(t, getErr)
	require.Equal(t, domain.UsageSettlementStateProcessing, current.State)
}
