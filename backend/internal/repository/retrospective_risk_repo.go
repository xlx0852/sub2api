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
)

type retrospectiveRiskRepository struct {
	db *sql.DB
}

func NewRetrospectiveRiskRepository(db *sql.DB) ports.RetrospectiveRiskRepository {
	return &retrospectiveRiskRepository{db: db}
}

func (r *retrospectiveRiskRepository) ReserveEvidence(ctx context.Context, item *domain.RetrospectiveEvidence, maxItems int, maxBytes int64) (bool, error) {
	if r == nil || r.db == nil || item == nil || item.UserID <= 0 {
		return false, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin retrospective evidence reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	lockKey := fmt.Sprintf("retrospective-evidence:%d:%s", item.UserID, item.CaptureDay.Format("2006-01-02"))
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
		return false, fmt.Errorf("lock retrospective evidence budget: %w", err)
	}
	var count int
	var bytesUsed int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(SUM(plaintext_bytes), 0)
FROM content_moderation_evidence
WHERE user_id = $1 AND capture_day = $2
  AND status NOT IN ('failed', 'expired', 'deleted')
`, item.UserID, item.CaptureDay).Scan(&count, &bytesUsed); err != nil {
		return false, fmt.Errorf("read retrospective evidence budget: %w", err)
	}
	if count >= maxItems || bytesUsed+item.PlaintextBytes > maxBytes {
		return false, nil
	}
	var id string
	err = tx.QueryRowContext(ctx, `
INSERT INTO content_moderation_evidence (
    id, user_id, api_key_id, request_id, endpoint, provider, model, content_hash,
    capture_day, capture_reason, object_key, key_version, status, plaintext_bytes,
    truncated, redacted_excerpt, captured_at, expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'reserved',$13,$14,$15,$16,$17)
ON CONFLICT DO NOTHING
RETURNING id
`, item.ID, item.UserID, nullableRetrospectiveInt64(item.APIKeyID), item.RequestID, item.Endpoint, item.Provider, item.Model,
		item.ContentHash, item.CaptureDay, item.CaptureReason, item.ObjectKey, item.KeyVersion, item.PlaintextBytes,
		item.Truncated, item.RedactedExcerpt, item.CapturedAt, item.ExpiresAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reserve retrospective evidence: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit retrospective evidence reservation: %w", err)
	}
	return true, nil
}

func (r *retrospectiveRiskRepository) MarkEvidenceStored(ctx context.Context, id string, storedBytes int64) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE content_moderation_evidence
SET status='stored', stored_bytes=$2, stored_at=NOW(), last_error=''
WHERE id=$1 AND status IN ('reserved','failed')
`, id, storedBytes)
	if err != nil {
		return fmt.Errorf("mark retrospective evidence stored: %w", err)
	}
	return nil
}

func (r *retrospectiveRiskRepository) MarkEvidenceFailed(ctx context.Context, id string, reason string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE content_moderation_evidence
SET status='failed', last_error=$2
WHERE id=$1 AND status IN ('reserved','stored','reviewing')
`, id, truncateDBText(reason, 1024))
	if err != nil {
		return fmt.Errorf("mark retrospective evidence failed: %w", err)
	}
	return nil
}

func (r *retrospectiveRiskRepository) MoveEvidenceObject(ctx context.Context, id, objectKey string, storedBytes int64, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE content_moderation_evidence
SET object_key=$2, stored_bytes=$3, expires_at=$4
WHERE id=$1
`, id, objectKey, storedBytes, expiresAt)
	if err != nil {
		return fmt.Errorf("move retrospective evidence object metadata: %w", err)
	}
	return nil
}

func (r *retrospectiveRiskRepository) ListExpiredEvidence(ctx context.Context, now time.Time, limit int) ([]domain.RetrospectiveEvidence, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id,object_key,key_version,status,expires_at FROM content_moderation_evidence
WHERE expires_at <= $1 AND status NOT IN ('expired','deleted')
ORDER BY expires_at,id LIMIT $2
`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []domain.RetrospectiveEvidence{}
	for rows.Next() {
		var item domain.RetrospectiveEvidence
		if err := rows.Scan(&item.ID, &item.ObjectKey, &item.KeyVersion, &item.Status, &item.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *retrospectiveRiskRepository) MarkEvidenceDeleted(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE content_moderation_evidence SET status='deleted',last_error='' WHERE id=$1`, id)
	return err
}

func (r *retrospectiveRiskRepository) EvidenceObjectExists(ctx context.Context, objectKey string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM content_moderation_evidence WHERE object_key=$1 AND status<>'deleted')`, objectKey).Scan(&exists)
	return exists, err
}

func (r *retrospectiveRiskRepository) CreateReviewRun(ctx context.Context, runKey string, scheduledDay time.Time, manual bool, policyVersion string, configSnapshot []byte) (*domain.RetrospectiveReviewRun, bool, error) {
	if len(configSnapshot) == 0 || !json.Valid(configSnapshot) {
		configSnapshot = []byte(`{}`)
	}
	run := &domain.RetrospectiveReviewRun{}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO content_moderation_review_runs (run_key, scheduled_day, manual, policy_version, config_snapshot)
VALUES ($1,$2,$3,$4,$5::jsonb)
ON CONFLICT (run_key) DO NOTHING
RETURNING id, run_key, scheduled_day, status, manual, policy_version,
          claimed_count, reviewed_count, flagged_count, failed_count, input_tokens,
          cost_units::float8, error, started_at, heartbeat_at, finished_at
	`, runKey, scheduledDay, manual, policyVersion, string(configSnapshot)).Scan(reviewRunScanArgs(run)...)
	if errors.Is(err, sql.ErrNoRows) {
		// The advisory lock is held by the caller, so a prior running/failed row can
		// only belong to an interrupted worker. Resume it instead of leaving the
		// day's run permanently stuck after a process restart.
		err = r.db.QueryRowContext(ctx, `
UPDATE content_moderation_review_runs
SET status='running', error='', heartbeat_at=NOW(), finished_at=NULL
WHERE run_key=$1 AND status IN ('running','failed')
RETURNING id, run_key, scheduled_day, status, manual, policy_version,
          claimed_count, reviewed_count, flagged_count, failed_count, input_tokens,
          cost_units::float8, error, started_at, heartbeat_at, finished_at
`, runKey).Scan(reviewRunScanArgs(run)...)
		if err == nil {
			return run, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, fmt.Errorf("resume retrospective review run: %w", err)
		}
		err = r.db.QueryRowContext(ctx, `
SELECT id, run_key, scheduled_day, status, manual, policy_version,
       claimed_count, reviewed_count, flagged_count, failed_count, input_tokens,
       cost_units::float8, error, started_at, heartbeat_at, finished_at
FROM content_moderation_review_runs WHERE run_key=$1
`, runKey).Scan(reviewRunScanArgs(run)...)
		return run, false, err
	}
	if err != nil {
		return nil, false, fmt.Errorf("create retrospective review run: %w", err)
	}
	return run, true, nil
}

func (r *retrospectiveRiskRepository) TryReviewLock(ctx context.Context) (func(), bool, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("open retrospective review lock connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext('content_moderation_retrospective_review'))`).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("acquire retrospective review lock: %w", err)
	}
	if !acquired {
		_ = conn.Close()
		return func() {}, false, nil
	}
	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext('content_moderation_retrospective_review'))`)
		_ = conn.Close()
	}
	return release, true, nil
}

func (r *retrospectiveRiskRepository) ClaimEvidence(ctx context.Context, runID int64, policyVersion string, limit int) ([]domain.RetrospectiveEvidence, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
WITH candidates AS (
    SELECT e.id
    FROM content_moderation_evidence e
    WHERE (e.status='stored' OR (e.status='reviewing' AND e.claimed_at < NOW() - INTERVAL '15 minutes'))
      AND e.expires_at > NOW()
      AND NOT EXISTS (
          SELECT 1 FROM content_moderation_review_items i
          WHERE i.evidence_id=e.id AND i.policy_version=$2 AND i.status='completed'
      )
    ORDER BY e.captured_at, e.id
    FOR UPDATE SKIP LOCKED
    LIMIT $3
), claimed AS (
    UPDATE content_moderation_evidence e
    SET status='reviewing', claimed_at=NOW(), review_attempts=review_attempts+1
    FROM candidates c
    WHERE e.id=c.id
    RETURNING e.*
)
SELECT c.id, c.user_id, u.email, c.api_key_id, c.request_id, c.endpoint, c.provider, c.model,
       c.content_hash, c.capture_day, c.capture_reason, c.object_key, c.key_version, c.status,
       c.plaintext_bytes, c.stored_bytes, c.truncated, c.redacted_excerpt, c.review_attempts,
       c.last_error, c.captured_at, c.stored_at, c.claimed_at, c.reviewed_at, c.expires_at
FROM claimed c
LEFT JOIN users u ON u.id=c.user_id
ORDER BY c.captured_at, c.id
`, runID, policyVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("claim retrospective evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]domain.RetrospectiveEvidence, 0)
	for rows.Next() {
		var item domain.RetrospectiveEvidence
		var apiKeyID sql.NullInt64
		var storedAt, claimedAt, reviewedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.UserEmail, &apiKeyID, &item.RequestID, &item.Endpoint, &item.Provider, &item.Model,
			&item.ContentHash, &item.CaptureDay, &item.CaptureReason, &item.ObjectKey, &item.KeyVersion, &item.Status,
			&item.PlaintextBytes, &item.StoredBytes, &item.Truncated, &item.RedactedExcerpt, &item.ReviewAttempts,
			&item.LastError, &item.CapturedAt, &storedAt, &claimedAt, &reviewedAt, &item.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan retrospective evidence: %w", err)
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if storedAt.Valid {
			v := storedAt.Time
			item.StoredAt = &v
		}
		if claimedAt.Valid {
			v := claimedAt.Time
			item.ClaimedAt = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.Time
			item.ReviewedAt = &v
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *retrospectiveRiskRepository) CompleteReviewItem(ctx context.Context, runID int64, policyVersion string, evidence domain.RetrospectiveEvidence, result domain.RetrospectiveReviewResult) (int64, error) {
	reasons, _ := json.Marshal(result.ReasonCodes)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var itemID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO content_moderation_review_items (
    run_id, evidence_id, policy_version, status, flagged, highest_category,
    highest_score, reason_codes, redacted_summary, input_tokens, cost_units, finished_at
) VALUES ($1,$2,$3,'completed',$4,$5,$6,$7::jsonb,$8,$9,$10,NOW())
ON CONFLICT (evidence_id, policy_version) DO UPDATE SET
    status='completed', flagged=EXCLUDED.flagged, highest_category=EXCLUDED.highest_category,
    highest_score=EXCLUDED.highest_score, reason_codes=EXCLUDED.reason_codes,
    redacted_summary=EXCLUDED.redacted_summary, input_tokens=EXCLUDED.input_tokens,
    cost_units=EXCLUDED.cost_units, error='', finished_at=NOW()
RETURNING id
`, runID, evidence.ID, policyVersion, result.Flagged, result.HighestCategory, result.HighestScore,
		string(reasons), truncateDBText(result.RedactedSummary, 1024), result.InputTokens, result.CostUnits).Scan(&itemID)
	if err != nil {
		return 0, fmt.Errorf("complete retrospective review item: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE content_moderation_evidence SET status='reviewed', reviewed_at=NOW(), last_error='' WHERE id=$1
`, evidence.ID); err != nil {
		return 0, fmt.Errorf("mark retrospective evidence reviewed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return itemID, nil
}

func (r *retrospectiveRiskRepository) FailReviewItem(ctx context.Context, runID int64, evidenceID, policyVersion, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO content_moderation_review_items (run_id, evidence_id, policy_version, status, error, finished_at)
VALUES ($1,$2,$3,'failed',$4,NOW())
ON CONFLICT (evidence_id, policy_version) DO UPDATE SET status='failed', error=EXCLUDED.error, finished_at=NOW()
`, runID, evidenceID, policyVersion, truncateDBText(reason, 2048))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE content_moderation_evidence
SET status=CASE WHEN $2 LIKE 'terminal:%' OR review_attempts >= 3 THEN 'failed' ELSE 'stored' END,
    last_error=$2
WHERE id=$1
`, evidenceID, truncateDBText(reason, 1024))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *retrospectiveRiskRepository) UpdateReviewRun(ctx context.Context, run *domain.RetrospectiveReviewRun) error {
	if run == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE content_moderation_review_runs SET
 status=$2, claimed_count=$3, reviewed_count=$4, flagged_count=$5, failed_count=$6,
 input_tokens=$7, cost_units=$8, error=$9, heartbeat_at=NOW(),
 finished_at=CASE WHEN $2 IN ('completed','budget_limited','failed') THEN COALESCE(finished_at,NOW()) ELSE NULL END
WHERE id=$1
`, run.ID, run.Status, run.ClaimedCount, run.ReviewedCount, run.FlaggedCount, run.FailedCount,
		run.InputTokens, run.CostUnits, truncateDBText(run.Error, 2048))
	if err != nil {
		return fmt.Errorf("update retrospective review run: %w", err)
	}
	return nil
}

func (r *retrospectiveRiskRepository) LatestReviewRun(ctx context.Context) (*domain.RetrospectiveReviewRun, error) {
	run := &domain.RetrospectiveReviewRun{}
	err := r.db.QueryRowContext(ctx, `
SELECT id, run_key, scheduled_day, status, manual, policy_version,
       claimed_count, reviewed_count, flagged_count, failed_count, input_tokens,
       cost_units::float8, error, started_at, heartbeat_at, finished_at
FROM content_moderation_review_runs ORDER BY started_at DESC, id DESC LIMIT 1
`).Scan(reviewRunScanArgs(run)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest retrospective review run: %w", err)
	}
	return run, nil
}

func (r *retrospectiveRiskRepository) ListReviewRuns(ctx context.Context, limit, offset int) ([]domain.RetrospectiveReviewRun, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_moderation_review_runs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id,run_key,scheduled_day,status,manual,policy_version,claimed_count,reviewed_count,
       flagged_count,failed_count,input_tokens,cost_units::float8,error,started_at,heartbeat_at,finished_at
FROM content_moderation_review_runs ORDER BY started_at DESC,id DESC LIMIT $1 OFFSET $2
`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []domain.RetrospectiveReviewRun{}
	for rows.Next() {
		var run domain.RetrospectiveReviewRun
		if err := rows.Scan(reviewRunScanArgs(&run)...); err != nil {
			return nil, 0, err
		}
		items = append(items, run)
	}
	return items, total, rows.Err()
}

func (r *retrospectiveRiskRepository) EvidenceStats(ctx context.Context) (int64, int64, error) {
	var items, storedBytes int64
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*),COALESCE(SUM(stored_bytes),0) FROM content_moderation_evidence
WHERE status IN ('stored','reviewing','reviewed') AND expires_at>NOW()
`).Scan(&items, &storedBytes)
	return items, storedBytes, err
}

func reviewRunScanArgs(run *domain.RetrospectiveReviewRun) []any {
	return []any{
		&run.ID, &run.RunKey, &run.ScheduledDay, &run.Status, &run.Manual, &run.PolicyVersion,
		&run.ClaimedCount, &run.ReviewedCount, &run.FlaggedCount, &run.FailedCount,
		&run.InputTokens, &run.CostUnits, &run.Error, &run.StartedAt, &run.HeartbeatAt, &run.FinishedAt,
	}
}

func (r *retrospectiveRiskRepository) UpsertRiskProfileFromFinding(ctx context.Context, evidence domain.RetrospectiveEvidence, reviewItemID int64, result domain.RetrospectiveReviewResult, ttl time.Duration, activate bool) (*domain.UserRiskProfile, error) {
	if !result.Flagged || !activate {
		return nil, nil
	}
	reasons, _ := json.Marshal(result.ReasonCodes)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, fmt.Sprintf("retrospective-risk:%d", evidence.UserID)); err != nil {
		return nil, err
	}
	previousLevel := domain.RiskLevelNormal
	var existingLevel string
	_ = tx.QueryRowContext(ctx, `SELECT level FROM content_moderation_user_risk_profiles WHERE user_id=$1`, evidence.UserID).Scan(&existingLevel)
	if existingLevel != "" {
		previousLevel = existingLevel
	}
	var priorFindings int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM content_moderation_risk_events
WHERE user_id=$1 AND event_type='offline_finding' AND created_at > NOW() - INTERVAL '30 days'
`, evidence.UserID).Scan(&priorFindings); err != nil {
		return nil, err
	}
	level := retrospectiveRiskLevel(priorFindings, result.HighestScore)
	expiresAt := time.Now().Add(ttl)
	eventKey := fmt.Sprintf("offline-finding:%s:%d", evidence.ID, reviewItemID)
	var eventID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO content_moderation_risk_events (
 event_key, user_id, evidence_id, review_item_id, event_type, previous_level, new_level,
 confidence, reason_codes, source
) VALUES ($1,$2,$3,$4,'offline_finding',$5,$6,$7,$8::jsonb,'retrospective')
ON CONFLICT (event_key) DO UPDATE SET event_key=EXCLUDED.event_key
RETURNING id
`, eventKey, evidence.UserID, evidence.ID, reviewItemID, previousLevel, level, result.HighestScore, string(reasons)).Scan(&eventID)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO content_moderation_user_risk_profiles (
 user_id, level, score, confidence, reason_codes, provenance, review_status, expires_at, last_event_at
) VALUES ($1,$2,$3,$4,$5::jsonb,'retrospective','unreviewed',$6,NOW())
ON CONFLICT (user_id) DO UPDATE SET
 level=CASE
   WHEN content_moderation_user_risk_profiles.level='critical' THEN 'critical'
   WHEN content_moderation_user_risk_profiles.level='high' AND EXCLUDED.level='watch' THEN 'high'
   ELSE EXCLUDED.level END,
 score=GREATEST(content_moderation_user_risk_profiles.score, EXCLUDED.score),
 confidence=GREATEST(content_moderation_user_risk_profiles.confidence, EXCLUDED.confidence),
 reason_codes=EXCLUDED.reason_codes, provenance='retrospective', review_status='unreviewed',
 expires_at=GREATEST(content_moderation_user_risk_profiles.expires_at, EXCLUDED.expires_at),
 last_event_at=NOW(), updated_at=NOW()
`, evidence.UserID, level, result.HighestScore, result.HighestScore, string(reasons), expiresAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getRiskProfile(ctx, evidence.UserID)
}

func retrospectiveRiskLevel(priorFindings int, score float64) string {
	// priorFindings excludes the current event. A single offline finding is
	// monitoring-only regardless of score.
	if priorFindings >= 2 && score >= 0.95 {
		return domain.RiskLevelCritical
	}
	if priorFindings >= 1 {
		return domain.RiskLevelHigh
	}
	return domain.RiskLevelWatch
}

func (r *retrospectiveRiskRepository) ListEvidenceCases(ctx context.Context, userID int64, status string, limit, offset int) ([]domain.RetrospectiveEvidenceCase, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := "TRUE"
	args := []any{}
	if userID > 0 {
		args = append(args, userID)
		where += fmt.Sprintf(" AND e.user_id=$%d", len(args))
	}
	if status = strings.TrimSpace(status); status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND e.status=$%d", len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_moderation_evidence e WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
SELECT e.id,e.user_id,COALESCE(u.email,''),e.api_key_id,e.request_id,e.endpoint,e.provider,e.model,
 e.content_hash,e.capture_day,e.capture_reason,e.object_key,e.key_version,e.status,e.plaintext_bytes,
 e.stored_bytes,e.truncated,e.redacted_excerpt,e.review_attempts,e.last_error,e.captured_at,e.stored_at,
 e.claimed_at,e.reviewed_at,e.expires_at,COALESCE(i.status,''),COALESCE(i.flagged,false),
 COALESCE(i.highest_category,''),COALESCE(i.highest_score,0),COALESCE(i.reason_codes,'[]'::jsonb),
 COALESCE(i.redacted_summary,'')
FROM content_moderation_evidence e
LEFT JOIN users u ON u.id=e.user_id
LEFT JOIN LATERAL (
 SELECT status,flagged,highest_category,highest_score,reason_codes,redacted_summary
 FROM content_moderation_review_items WHERE evidence_id=e.id ORDER BY id DESC LIMIT 1
) i ON TRUE
WHERE `+where+` ORDER BY e.captured_at DESC,e.id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []domain.RetrospectiveEvidenceCase{}
	for rows.Next() {
		var item domain.RetrospectiveEvidenceCase
		var apiKeyID sql.NullInt64
		var storedAt, claimedAt, reviewedAt sql.NullTime
		var reasons []byte
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserEmail, &apiKeyID, &item.RequestID, &item.Endpoint, &item.Provider, &item.Model,
			&item.ContentHash, &item.CaptureDay, &item.CaptureReason, &item.ObjectKey, &item.KeyVersion, &item.Status, &item.PlaintextBytes,
			&item.StoredBytes, &item.Truncated, &item.RedactedExcerpt, &item.ReviewAttempts, &item.LastError, &item.CapturedAt, &storedAt,
			&claimedAt, &reviewedAt, &item.ExpiresAt, &item.ReviewStatus, &item.Flagged, &item.HighestCategory, &item.HighestScore, &reasons,
			&item.RedactedSummary); err != nil {
			return nil, 0, err
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if storedAt.Valid {
			v := storedAt.Time
			item.StoredAt = &v
		}
		if claimedAt.Valid {
			v := claimedAt.Time
			item.ClaimedAt = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.Time
			item.ReviewedAt = &v
		}
		_ = json.Unmarshal(reasons, &item.ReasonCodes)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *retrospectiveRiskRepository) GetEvidence(ctx context.Context, evidenceID string) (*domain.RetrospectiveEvidence, error) {
	var item domain.RetrospectiveEvidence
	var apiKeyID sql.NullInt64
	var storedAt, claimedAt, reviewedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT id,user_id,api_key_id,request_id,endpoint,provider,model,content_hash,capture_day,capture_reason,
 object_key,key_version,status,plaintext_bytes,stored_bytes,truncated,redacted_excerpt,review_attempts,last_error,
 captured_at,stored_at,claimed_at,reviewed_at,expires_at
FROM content_moderation_evidence WHERE id=$1`, evidenceID).Scan(
		&item.ID, &item.UserID, &apiKeyID, &item.RequestID, &item.Endpoint, &item.Provider, &item.Model, &item.ContentHash,
		&item.CaptureDay, &item.CaptureReason, &item.ObjectKey, &item.KeyVersion, &item.Status, &item.PlaintextBytes,
		&item.StoredBytes, &item.Truncated, &item.RedactedExcerpt, &item.ReviewAttempts, &item.LastError, &item.CapturedAt,
		&storedAt, &claimedAt, &reviewedAt, &item.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		item.APIKeyID = &v
	}
	if storedAt.Valid {
		v := storedAt.Time
		item.StoredAt = &v
	}
	if claimedAt.Valid {
		v := claimedAt.Time
		item.ClaimedAt = &v
	}
	if reviewedAt.Valid {
		v := reviewedAt.Time
		item.ReviewedAt = &v
	}
	return &item, nil
}

func (r *retrospectiveRiskRepository) CreateEvidenceAccessGrant(ctx context.Context, evidenceID string, adminID int64, tokenHash, reason string, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO content_moderation_evidence_access_grants
(evidence_id,admin_id,token_hash,reason,expires_at) VALUES ($1,$2,$3,$4,$5)`, evidenceID, adminID, tokenHash, truncateDBText(reason, 512), expiresAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO content_moderation_operator_audits
(admin_id,action,evidence_id,user_id,reason,outcome)
SELECT $2,'evidence_authorize',$1::varchar(36),user_id,$3,'granted'
FROM content_moderation_evidence WHERE id=$1::varchar(36)`, evidenceID, adminID, truncateDBText(reason, 512)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *retrospectiveRiskRepository) ConsumeEvidenceAccessGrant(ctx context.Context, evidenceID string, adminID int64, tokenHash string, now time.Time) (*domain.EvidenceAccessGrant, error) {
	grant := &domain.EvidenceAccessGrant{}
	err := r.db.QueryRowContext(ctx, `UPDATE content_moderation_evidence_access_grants
SET consumed_at=$4 WHERE evidence_id=$1 AND admin_id=$2 AND token_hash=$3 AND consumed_at IS NULL AND expires_at>$4
RETURNING id,evidence_id,admin_id,reason,expires_at,consumed_at`, evidenceID, adminID, tokenHash, now).Scan(
		&grant.ID, &grant.EvidenceID, &grant.AdminID, &grant.Reason, &grant.ExpiresAt, &grant.ConsumedAt)
	return grant, err
}

func (r *retrospectiveRiskRepository) AuditOperatorAction(ctx context.Context, adminID int64, action, evidenceID string, userID int64, reason, outcome string) error {
	var evidence any
	if strings.TrimSpace(evidenceID) != "" {
		evidence = evidenceID
	}
	var user any
	if userID > 0 {
		user = userID
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO content_moderation_operator_audits
(admin_id,action,evidence_id,user_id,reason,outcome) VALUES ($1,$2,$3,$4,$5,$6)`, adminID, action, evidence, user, truncateDBText(reason, 512), outcome)
	return err
}

func (r *retrospectiveRiskRepository) ListRiskProfiles(ctx context.Context, level string, limit, offset int) ([]domain.UserRiskProfile, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := "p.level <> 'normal' AND (p.expires_at IS NULL OR p.expires_at > NOW())"
	args := []any{}
	if level = strings.TrimSpace(level); level != "" {
		args = append(args, level)
		where += fmt.Sprintf(" AND p.level=$%d", len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_moderation_user_risk_profiles p WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
SELECT p.user_id, COALESCE(u.email,''), COALESCE(u.username,''), p.level, p.score, p.confidence,
       p.reason_codes, p.provenance, p.review_status, p.expires_at, p.last_event_at, p.created_at, p.updated_at
FROM content_moderation_user_risk_profiles p LEFT JOIN users u ON u.id=p.user_id
WHERE `+where+` ORDER BY p.updated_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []domain.UserRiskProfile{}
	for rows.Next() {
		item, err := scanRiskProfile(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *retrospectiveRiskRepository) ListRiskEvents(ctx context.Context, userID int64, limit, offset int) ([]domain.RiskEvent, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := "TRUE"
	args := []any{}
	if userID > 0 {
		args = append(args, userID)
		where = fmt.Sprintf("user_id=$%d", len(args))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_moderation_risk_events WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
SELECT id,event_key,user_id,evidence_id,review_item_id,event_type,previous_level,new_level,
       confidence,reason_codes,source,actor_admin_id,note,created_at
FROM content_moderation_risk_events WHERE `+where+`
ORDER BY created_at DESC,id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []domain.RiskEvent{}
	for rows.Next() {
		var item domain.RiskEvent
		var evidenceID sql.NullString
		var reviewItemID, adminID sql.NullInt64
		var reasons []byte
		if err := rows.Scan(&item.ID, &item.EventKey, &item.UserID, &evidenceID, &reviewItemID, &item.EventType, &item.PreviousLevel, &item.NewLevel, &item.Confidence, &reasons, &item.Source, &adminID, &item.Note, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		if evidenceID.Valid {
			item.EvidenceID = evidenceID.String
		}
		if reviewItemID.Valid {
			v := reviewItemID.Int64
			item.ReviewItemID = &v
		}
		if adminID.Valid {
			v := adminID.Int64
			item.ActorAdminID = &v
		}
		_ = json.Unmarshal(reasons, &item.ReasonCodes)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *retrospectiveRiskRepository) SetRiskProfile(ctx context.Context, userID int64, level, reviewStatus, note string, adminID *int64, ttl time.Duration) (*domain.UserRiskProfile, error) {
	level = strings.TrimSpace(level)
	if level == "" {
		level = domain.RiskLevelNormal
	}
	if reviewStatus == "" {
		reviewStatus = "unreviewed"
	}
	previous := domain.RiskLevelNormal
	_ = r.db.QueryRowContext(ctx, `SELECT level FROM content_moderation_user_risk_profiles WHERE user_id=$1`, userID).Scan(&previous)
	var expiresAt any
	if level != domain.RiskLevelNormal && ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO content_moderation_user_risk_profiles (user_id,level,review_status,provenance,expires_at,last_event_at)
VALUES ($1,$2,$3,'admin',$4,NOW())
ON CONFLICT (user_id) DO UPDATE SET level=EXCLUDED.level,review_status=EXCLUDED.review_status,
 score=CASE WHEN EXCLUDED.level='normal' THEN 0 ELSE content_moderation_user_risk_profiles.score END,
 confidence=CASE WHEN EXCLUDED.level='normal' THEN 0 ELSE content_moderation_user_risk_profiles.confidence END,
 reason_codes=CASE WHEN EXCLUDED.level='normal' THEN '[]'::jsonb ELSE content_moderation_user_risk_profiles.reason_codes END,
 provenance='admin',expires_at=EXCLUDED.expires_at,last_event_at=NOW(),updated_at=NOW()
`, userID, level, reviewStatus, expiresAt)
	if err != nil {
		return nil, err
	}
	eventKey := fmt.Sprintf("admin-risk:%d:%d", userID, time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `
INSERT INTO content_moderation_risk_events (event_key,user_id,event_type,previous_level,new_level,source,actor_admin_id,note)
VALUES ($1,$2,'admin_review',$3,$4,'admin',$5,$6)
`, eventKey, userID, previous, level, nullableRetrospectiveInt64(adminID), truncateDBText(note, 1024))
	if err != nil {
		return nil, err
	}
	if adminID != nil {
		if _, err = tx.ExecContext(ctx, `INSERT INTO content_moderation_operator_audits
(admin_id,action,user_id,reason,outcome) VALUES ($1,'risk_profile_review',$2,$3,'success')`, *adminID, userID, truncateDBText(note, 512)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getRiskProfile(ctx, userID)
}

func (r *retrospectiveRiskRepository) CountActiveRiskProfiles(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_moderation_user_risk_profiles WHERE level <> 'normal' AND (expires_at IS NULL OR expires_at > NOW())`).Scan(&count)
	return count, err
}

func (r *retrospectiveRiskRepository) IsRiskElevated(ctx context.Context, userID int64, now time.Time) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var elevated bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(
 SELECT 1 FROM content_moderation_user_risk_profiles
 WHERE user_id=$1 AND level IN ('watch','high','critical')
   AND (expires_at IS NULL OR expires_at > $2)
)
`, userID, now).Scan(&elevated)
	return elevated, err
}

func (r *retrospectiveRiskRepository) ExpireRiskProfiles(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
WITH due AS (
 SELECT user_id FROM content_moderation_user_risk_profiles
 WHERE level <> 'normal' AND expires_at <= $1 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED
), updated AS (
 UPDATE content_moderation_user_risk_profiles p SET level='normal',score=0,confidence=0,
  reason_codes='[]'::jsonb,review_status='cleared',updated_at=NOW()
 FROM due WHERE p.user_id=due.user_id RETURNING p.user_id
)
SELECT user_id FROM updated
`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *retrospectiveRiskRepository) getRiskProfile(ctx context.Context, userID int64) (*domain.UserRiskProfile, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT p.user_id,COALESCE(u.email,''),COALESCE(u.username,''),p.level,p.score,p.confidence,
       p.reason_codes,p.provenance,p.review_status,p.expires_at,p.last_event_at,p.created_at,p.updated_at
FROM content_moderation_user_risk_profiles p LEFT JOIN users u ON u.id=p.user_id WHERE p.user_id=$1
`, userID)
	return scanRiskProfile(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRiskProfile(row rowScanner) (*domain.UserRiskProfile, error) {
	var item domain.UserRiskProfile
	var reasons []byte
	var expiresAt, lastEventAt sql.NullTime
	if err := row.Scan(&item.UserID, &item.Email, &item.Username, &item.Level, &item.Score, &item.Confidence, &reasons, &item.Provenance, &item.ReviewStatus, &expiresAt, &lastEventAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(reasons, &item.ReasonCodes)
	if expiresAt.Valid {
		v := expiresAt.Time
		item.ExpiresAt = &v
	}
	if lastEventAt.Valid {
		v := lastEventAt.Time
		item.LastEventAt = &v
	}
	return &item, nil
}

func nullableRetrospectiveInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func truncateDBText(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}
