package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/lib/pq"
)

const usageSettlementSelectColumns = `
	id, billing_identity, api_key_id, request_fingerprint,
	user_id, account_id, group_id, provider, request_type,
	provider_request_id, inbound_endpoint, upstream_endpoint,
	recovery_context_version, recovery_context,
	authoritative,
	state, payload_version, payload, payload_hash,
	lease_owner, lease_expires_at, retry_count, next_attempt_at,
	reconciliation_deadline, last_error_code, last_error_message,
	started_at, ready_at, settled_at, created_at, updated_at
`

type usageSettlementRepository struct {
	db *sql.DB
}

func NewUsageSettlementRepository(sqlDB *sql.DB) ports.UsageSettlementRepository {
	return &usageSettlementRepository{db: sqlDB}
}

func (r *usageSettlementRepository) CreateStarted(ctx context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("usage settlement repository db is nil")
	}
	if err := attempt.ValidateStarted(); err != nil {
		return nil, false, err
	}
	startedAt := attempt.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	var recoveryContext any
	var recoveryContextVersion any
	if attempt.RecoveryContext != nil {
		encoded, err := json.Marshal(attempt.RecoveryContext)
		if err != nil {
			return nil, false, err
		}
		recoveryContext = encoded
		recoveryContextVersion = attempt.RecoveryContext.Version
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO usage_settlement_attempts (
			billing_identity, api_key_id, request_fingerprint,
			user_id, account_id, group_id, provider, request_type,
			provider_request_id, inbound_endpoint, upstream_endpoint,
			recovery_context_version, recovery_context,
			authoritative,
			state, payload_version, reconciliation_deadline, started_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13::jsonb, $14, $15, $16, $17, $18)
		ON CONFLICT (billing_identity, api_key_id) DO NOTHING
		RETURNING `+usageSettlementSelectColumns,
		attempt.BillingIdentity, attempt.APIKeyID, attempt.RequestFingerprint,
		attempt.UserID, attempt.AccountID, attempt.GroupID, attempt.Provider, int16(attempt.RequestType),
		attempt.ProviderRequestID, attempt.InboundEndpoint, attempt.UpstreamEndpoint,
		recoveryContextVersion, recoveryContext,
		attempt.Authoritative, domain.UsageSettlementStateStarted, domain.UsageSettlementPayloadVersionV1,
		attempt.ReconciliationDeadline, startedAt,
	)
	created, err := scanUsageSettlementAttempt(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	existing, err := r.GetByIdentity(ctx, attempt.BillingIdentity, attempt.APIKeyID)
	if err != nil {
		return nil, false, err
	}
	if existing.RequestFingerprint != attempt.RequestFingerprint {
		return nil, false, domain.ErrUsageSettlementFingerprintConflict
	}
	return existing, false, nil
}

func (r *usageSettlementRepository) ObserveProviderRequestID(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, providerRequestID string) (*domain.UsageSettlementAttempt, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	billingIdentity = strings.TrimSpace(billingIdentity)
	requestFingerprint = strings.TrimSpace(requestFingerprint)
	providerRequestID = strings.TrimSpace(providerRequestID)
	if billingIdentity == "" {
		return nil, domain.ErrUsageSettlementIdentityRequired
	}
	if requestFingerprint == "" {
		return nil, domain.ErrUsageSettlementFingerprintRequired
	}
	if providerRequestID == "" {
		return nil, errors.New("usage settlement provider request id is required")
	}
	if len(providerRequestID) > 255 {
		return nil, errors.New("usage settlement provider request id exceeds 255 characters")
	}
	updated, err := scanUsageSettlementAttempt(r.db.QueryRowContext(ctx, `
		UPDATE usage_settlement_attempts
		SET provider_request_id = $1, updated_at = NOW()
		WHERE billing_identity = $2
			AND api_key_id = $3
			AND request_fingerprint = $4
			AND (provider_request_id IS NULL OR provider_request_id = '' OR provider_request_id = $1)
		RETURNING `+usageSettlementSelectColumns,
		providerRequestID, billingIdentity, apiKeyID, requestFingerprint,
	))
	if err == nil {
		return updated, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	existing, getErr := r.GetByIdentity(ctx, billingIdentity, apiKeyID)
	if getErr != nil {
		return nil, getErr
	}
	if existing.RequestFingerprint != requestFingerprint {
		return nil, domain.ErrUsageSettlementFingerprintConflict
	}
	if existing.ProviderRequestID != providerRequestID {
		return nil, domain.ErrUsageSettlementProviderIDConflict
	}
	return existing, nil
}

func (r *usageSettlementRepository) MarkReady(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint string, payload *domain.UsageSettlementPayloadV1) (_ *domain.UsageSettlementAttempt, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	billingIdentity = strings.TrimSpace(billingIdentity)
	requestFingerprint = strings.TrimSpace(requestFingerprint)
	if billingIdentity == "" {
		return nil, domain.ErrUsageSettlementIdentityRequired
	}
	if requestFingerprint == "" {
		return nil, domain.ErrUsageSettlementFingerprintRequired
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	attempt, err := getUsageSettlementByIdentity(ctx, tx, billingIdentity, apiKeyID, true)
	if err != nil {
		return nil, err
	}
	if attempt.RequestFingerprint != requestFingerprint {
		return nil, domain.ErrUsageSettlementFingerprintConflict
	}
	encoded, payloadHash, err := payload.CanonicalJSON(billingIdentity, apiKeyID, attempt.UserID, attempt.AccountID)
	if err != nil {
		return nil, err
	}
	if attempt.PayloadHash != "" {
		if attempt.PayloadHash != payloadHash {
			return nil, domain.ErrUsageSettlementPayloadConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		tx = nil
		return attempt, nil
	}
	if err := domain.ValidateUsageSettlementTransition(attempt.State, domain.UsageSettlementStateReady); err != nil {
		return nil, err
	}
	updated, err := scanUsageSettlementAttempt(tx.QueryRowContext(ctx, `
		UPDATE usage_settlement_attempts
		SET state = $1,
			payload_version = $2,
			payload = $3::jsonb,
			payload_hash = $4,
			ready_at = COALESCE(ready_at, NOW()),
			next_attempt_at = NOW(),
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error_code = NULL,
			last_error_message = NULL,
			updated_at = NOW()
		WHERE id = $5
		RETURNING `+usageSettlementSelectColumns,
		domain.UsageSettlementStateReady, payload.Version, encoded, payloadHash, attempt.ID,
	))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return updated, nil
}

func (r *usageSettlementRepository) Claim(ctx context.Context, request ports.UsageSettlementClaimRequest) ([]domain.UsageSettlementAttempt, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	request.Owner = strings.TrimSpace(request.Owner)
	if request.Owner == "" {
		return nil, errors.New("usage settlement claim owner is required")
	}
	if request.Limit <= 0 {
		request.Limit = 1
	}
	if request.Limit > 1000 {
		request.Limit = 1000
	}
	if request.LeaseDuration <= 0 {
		return nil, errors.New("usage settlement claim lease duration must be positive")
	}
	groupClause := ""
	args := []any{
		domain.UsageSettlementStateReady,
		domain.UsageSettlementStateFailedRetryable,
		domain.UsageSettlementStateProcessing,
		request.Limit,
		request.Owner,
		request.LeaseDuration.Milliseconds(),
	}
	if request.GroupIDs != nil {
		if len(request.GroupIDs) == 0 {
			return []domain.UsageSettlementAttempt{}, nil
		}
		for _, groupID := range request.GroupIDs {
			if groupID <= 0 {
				return nil, errors.New("usage settlement claim group IDs must be positive")
			}
		}
		groupClause = " AND group_id = ANY($7)"
		args = append(args, pq.Array(request.GroupIDs))
	}
	query := `
		WITH candidates AS (
			SELECT id
			FROM usage_settlement_attempts
			WHERE authoritative = TRUE
				AND (
				(state IN ($1, $2) AND (next_attempt_at IS NULL OR next_attempt_at <= NOW()))
				OR (state = $3 AND lease_expires_at <= NOW())
			)` + groupClause + `
			ORDER BY COALESCE(next_attempt_at, ready_at, created_at), id
			FOR UPDATE SKIP LOCKED
			LIMIT $4
		)
		UPDATE usage_settlement_attempts AS attempt
		SET state = $3,
			lease_owner = $5,
			lease_expires_at = NOW() + ($6 * INTERVAL '1 millisecond'),
			updated_at = NOW()
		FROM candidates
		WHERE attempt.id = candidates.id
		RETURNING ` + prefixedUsageSettlementColumns("attempt")
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]domain.UsageSettlementAttempt, 0, request.Limit)
	for rows.Next() {
		attempt, scanErr := scanUsageSettlementAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *usageSettlementRepository) ExpireStarted(ctx context.Context, now time.Time, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("usage settlement repository db is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	result, err := r.db.ExecContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM usage_settlement_attempts
			WHERE state = $1
				AND reconciliation_deadline IS NOT NULL
				AND reconciliation_deadline <= $2
			ORDER BY reconciliation_deadline, id
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE usage_settlement_attempts AS attempt
		SET state = $4,
			last_error_code = 'TERMINAL_USAGE_MISSING',
			last_error_message = 'complete terminal usage was not persisted before the reconciliation deadline',
			lease_owner = NULL,
			lease_expires_at = NULL,
			next_attempt_at = NULL,
			updated_at = NOW()
		FROM candidates
		WHERE attempt.id = candidates.id
	`, domain.UsageSettlementStateStarted, now.UTC(), limit, domain.UsageSettlementStateReconciliationRequired)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *usageSettlementRepository) ListExpiredRecoverable(ctx context.Context, now time.Time, limit int) ([]domain.UsageSettlementAttempt, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Claim overdue started attempts for one worker: bump the deadline so a
	// peer instance cannot recover the same provider request concurrently.
	const recoveryLease = 30 * time.Second
	rows, err := tx.QueryContext(ctx, `
			WITH candidates AS (
				SELECT id
				FROM usage_settlement_attempts
				WHERE state = $1
					AND reconciliation_deadline IS NOT NULL
					AND reconciliation_deadline <= $2
					AND provider_request_id IS NOT NULL
					AND provider_request_id <> ''
					AND recovery_context IS NOT NULL
				ORDER BY reconciliation_deadline, id
				FOR UPDATE SKIP LOCKED
				LIMIT $3
			)
			UPDATE usage_settlement_attempts AS attempt
			SET reconciliation_deadline = $4,
				updated_at = NOW()
			FROM candidates
			WHERE attempt.id = candidates.id
			RETURNING `+prefixedUsageSettlementColumns("attempt"),
		domain.UsageSettlementStateStarted, now.UTC(), limit, now.UTC().Add(recoveryLease))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]domain.UsageSettlementAttempt, 0, limit)
	for rows.Next() {
		attempt, scanErr := scanUsageSettlementAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *usageSettlementRepository) BacklogStats(ctx context.Context) (*ports.UsageSettlementBacklogStats, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	stats := &ports.UsageSettlementBacklogStats{}
	var oldest sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE state = $1),
			COUNT(*) FILTER (WHERE state = $2),
			COUNT(*) FILTER (WHERE state = $3),
			COUNT(*) FILTER (WHERE state = $4),
			COUNT(*) FILTER (WHERE state = $5),
			MIN(created_at) FILTER (WHERE state IN ($1, $2, $3, $4, $5))
		FROM usage_settlement_attempts
	`, domain.UsageSettlementStateStarted, domain.UsageSettlementStateReady,
		domain.UsageSettlementStateProcessing, domain.UsageSettlementStateFailedRetryable,
		domain.UsageSettlementStateReconciliationRequired,
	).Scan(&stats.Started, &stats.Ready, &stats.Processing, &stats.Retryable, &stats.ReconciliationRequired, &oldest)
	if err != nil {
		return nil, err
	}
	stats.OldestPendingAt = nullableTimePtr(oldest)
	return stats, nil
}

func (r *usageSettlementRepository) MarkRetryable(ctx context.Context, id int64, owner string, nextAttemptAt time.Time, code, message string) error {
	return r.markClaimedState(ctx, id, owner, domain.UsageSettlementStateFailedRetryable, nextAttemptAt, code, message)
}

func (r *usageSettlementRepository) MarkFailedTerminal(ctx context.Context, id int64, owner, code, message string) error {
	return r.markClaimedState(ctx, id, owner, domain.UsageSettlementStateFailedTerminal, time.Time{}, code, message)
}

func (r *usageSettlementRepository) markClaimedState(ctx context.Context, id int64, owner string, target domain.UsageSettlementState, nextAttemptAt time.Time, code, message string) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return domain.ErrUsageSettlementLeaseConflict
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE usage_settlement_attempts
		SET state = $1,
			retry_count = retry_count + CASE WHEN $2::boolean THEN 1 ELSE 0 END,
			next_attempt_at = CASE WHEN $2::boolean THEN $3::timestamptz ELSE NULL END,
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error_code = NULLIF($4, ''),
			last_error_message = NULLIF($5, ''),
			updated_at = NOW()
		WHERE id = $6 AND state = $7 AND lease_owner = $8
	`, target, target == domain.UsageSettlementStateFailedRetryable, nullableTime(nextAttemptAt), strings.TrimSpace(code), strings.TrimSpace(message), id, domain.UsageSettlementStateProcessing, owner)
	if err != nil {
		return err
	}
	return requireOneUsageSettlementRow(result)
}

func (r *usageSettlementRepository) MarkReconciliationRequired(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, code, message string) (_ error) {
	if r == nil || r.db == nil {
		return errors.New("usage settlement repository db is nil")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := getUsageSettlementByIdentity(ctx, tx, strings.TrimSpace(billingIdentity), apiKeyID, true)
	if err != nil {
		return err
	}
	if attempt.RequestFingerprint != strings.TrimSpace(requestFingerprint) {
		return domain.ErrUsageSettlementFingerprintConflict
	}
	if attempt.State == domain.UsageSettlementStateReconciliationRequired {
		return tx.Commit()
	}
	if err := domain.ValidateUsageSettlementTransition(attempt.State, domain.UsageSettlementStateReconciliationRequired); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE usage_settlement_attempts
		SET state = $1, lease_owner = NULL, lease_expires_at = NULL,
			last_error_code = NULLIF($2, ''), last_error_message = NULLIF($3, ''), updated_at = NOW()
		WHERE id = $4
	`, domain.UsageSettlementStateReconciliationRequired, strings.TrimSpace(code), strings.TrimSpace(message), attempt.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *usageSettlementRepository) MarkSettled(ctx context.Context, id int64, owner string) error {
	owner = strings.TrimSpace(owner)
	result, err := r.db.ExecContext(ctx, `
		UPDATE usage_settlement_attempts
		SET state = $1, settled_at = NOW(), lease_owner = NULL, lease_expires_at = NULL,
			next_attempt_at = NULL, last_error_code = NULL, last_error_message = NULL, updated_at = NOW()
		WHERE id = $2 AND state = $3 AND lease_owner = $4
	`, domain.UsageSettlementStateSettled, id, domain.UsageSettlementStateProcessing, owner)
	if err != nil {
		return err
	}
	if err := requireOneUsageSettlementRow(result); err == nil {
		return nil
	}
	var state string
	if queryErr := r.db.QueryRowContext(ctx, `SELECT state FROM usage_settlement_attempts WHERE id = $1`, id).Scan(&state); queryErr != nil {
		return domain.ErrUsageSettlementLeaseConflict
	}
	if domain.UsageSettlementState(state) == domain.UsageSettlementStateSettled {
		return nil
	}
	return domain.ErrUsageSettlementLeaseConflict
}

func (r *usageSettlementRepository) GetByIdentity(ctx context.Context, billingIdentity string, apiKeyID int64) (*domain.UsageSettlementAttempt, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage settlement repository db is nil")
	}
	return getUsageSettlementByIdentity(ctx, r.db, strings.TrimSpace(billingIdentity), apiKeyID, false)
}

type usageSettlementQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getUsageSettlementByIdentity(ctx context.Context, queryer usageSettlementQueryer, billingIdentity string, apiKeyID int64, forUpdate bool) (*domain.UsageSettlementAttempt, error) {
	query := `SELECT ` + usageSettlementSelectColumns + ` FROM usage_settlement_attempts WHERE billing_identity = $1 AND api_key_id = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanUsageSettlementAttempt(queryer.QueryRowContext(ctx, query, billingIdentity, apiKeyID))
}

func getUsageSettlementByID(ctx context.Context, queryer usageSettlementQueryer, id int64, forUpdate bool) (*domain.UsageSettlementAttempt, error) {
	query := `SELECT ` + usageSettlementSelectColumns + ` FROM usage_settlement_attempts WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanUsageSettlementAttempt(queryer.QueryRowContext(ctx, query, id))
}

type usageSettlementScanner interface {
	Scan(...any) error
}

func scanUsageSettlementAttempt(scanner usageSettlementScanner) (*domain.UsageSettlementAttempt, error) {
	var attempt domain.UsageSettlementAttempt
	var groupID sql.NullInt64
	var providerRequestID, inboundEndpoint, upstreamEndpoint sql.NullString
	var state string
	var recoveryContext, payload []byte
	var recoveryContextVersion sql.NullInt16
	var payloadHash, leaseOwner, lastErrorCode, lastErrorMessage sql.NullString
	var leaseExpiresAt, nextAttemptAt, reconciliationDeadline, readyAt, settledAt sql.NullTime
	var requestType int16
	err := scanner.Scan(
		&attempt.ID, &attempt.BillingIdentity, &attempt.APIKeyID, &attempt.RequestFingerprint,
		&attempt.UserID, &attempt.AccountID, &groupID, &attempt.Provider, &requestType,
		&providerRequestID, &inboundEndpoint, &upstreamEndpoint,
		&recoveryContextVersion, &recoveryContext,
		&attempt.Authoritative,
		&state, &attempt.PayloadVersion, &payload, &payloadHash,
		&leaseOwner, &leaseExpiresAt, &attempt.RetryCount, &nextAttemptAt,
		&reconciliationDeadline, &lastErrorCode, &lastErrorMessage,
		&attempt.StartedAt, &readyAt, &settledAt, &attempt.CreatedAt, &attempt.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	attempt.GroupID = nullableInt64Ptr(groupID)
	attempt.ProviderRequestID = providerRequestID.String
	if len(recoveryContext) > 0 {
		var decoded domain.UsageSettlementRecoveryContextV1
		if err := json.Unmarshal(recoveryContext, &decoded); err != nil {
			return nil, fmt.Errorf("decode usage settlement recovery context: %w", err)
		}
		if recoveryContextVersion.Valid && int(recoveryContextVersion.Int16) != decoded.Version {
			return nil, errors.New("usage settlement recovery context version mismatch")
		}
		attempt.RecoveryContext = &decoded
	}
	attempt.InboundEndpoint = inboundEndpoint.String
	attempt.UpstreamEndpoint = upstreamEndpoint.String
	attempt.State = domain.UsageSettlementState(state)
	attempt.PayloadHash = payloadHash.String
	attempt.LeaseOwner = leaseOwner.String
	attempt.LeaseExpiresAt = nullableTimePtr(leaseExpiresAt)
	attempt.NextAttemptAt = nullableTimePtr(nextAttemptAt)
	attempt.ReconciliationDeadline = nullableTimePtr(reconciliationDeadline)
	attempt.LastErrorCode = lastErrorCode.String
	attempt.LastErrorMessage = lastErrorMessage.String
	attempt.ReadyAt = nullableTimePtr(readyAt)
	attempt.SettledAt = nullableTimePtr(settledAt)
	attempt.RequestType = domain.RequestTypeFromInt16(requestType)
	if len(payload) > 0 {
		var decoded domain.UsageSettlementPayloadV1
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("decode usage settlement payload: %w", err)
		}
		attempt.Payload = &decoded
	}
	return &attempt, nil
}

func prefixedUsageSettlementColumns(alias string) string {
	parts := strings.Split(usageSettlementSelectColumns, ",")
	for index, part := range parts {
		parts[index] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func requireOneUsageSettlementRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrUsageSettlementLeaseConflict
	}
	return nil
}
