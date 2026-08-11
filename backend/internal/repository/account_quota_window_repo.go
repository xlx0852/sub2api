package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountQuotaWindowRepository struct {
	db *sql.DB
}

// NewAccountQuotaWindowRepository creates the quota-window ledger store.
func NewAccountQuotaWindowRepository(db *sql.DB) service.AccountQuotaWindowRepository {
	return &accountQuotaWindowRepository{db: db}
}

func (r *accountQuotaWindowRepository) ListByAccount(ctx context.Context, accountID int64, kind string, limit int) ([]*service.AccountQuotaWindow, error) {
	if limit <= 0 || limit > 200 {
		limit = 48
	}
	var (
		rows *sql.Rows
		err  error
	)
	if kind != "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, account_id, platform, kind, start_at, end_at, window_minutes, source,
			       COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at
			FROM account_quota_windows
			WHERE account_id = $1 AND kind = $2
			ORDER BY start_at DESC
			LIMIT $3`, accountID, kind, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, account_id, platform, kind, start_at, end_at, window_minutes, source,
			       COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at
			FROM account_quota_windows
			WHERE account_id = $1
			ORDER BY start_at DESC
			LIMIT $2`, accountID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccountQuotaWindows(rows)
}

func (r *accountQuotaWindowRepository) ListOpenByAccount(ctx context.Context, accountID int64) ([]*service.AccountQuotaWindow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, platform, kind, start_at, end_at, window_minutes, source,
		       COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at
		FROM account_quota_windows
		WHERE account_id = $1 AND is_open = TRUE
		ORDER BY kind`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccountQuotaWindows(rows)
}

func (r *accountQuotaWindowRepository) GetOpen(ctx context.Context, accountID int64, kind string) (*service.AccountQuotaWindow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, platform, kind, start_at, end_at, window_minutes, source,
		       COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at
		FROM account_quota_windows
		WHERE account_id = $1 AND kind = $2 AND is_open = TRUE
		LIMIT 1`, accountID, kind)
	w, err := scanAccountQuotaWindow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return w, err
}

func (r *accountQuotaWindowRepository) InsertOpen(ctx context.Context, w *service.AccountQuotaWindow) (*service.AccountQuotaWindow, error) {
	if w == nil {
		return nil, fmt.Errorf("nil window")
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_quota_windows (
			account_id, platform, kind, start_at, end_at, window_minutes, source,
			used_percent_open, is_open, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,TRUE,NOW(),NOW())
		RETURNING id, account_id, platform, kind, start_at, end_at, window_minutes, source,
		          COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at`,
		w.AccountID, w.Platform, w.Kind, w.StartAt, w.EndAt, w.WindowMinutes, nullIfEmpty(w.Source, service.QuotaWindowSourceSeed),
		w.UsedPercentOpen,
	)
	return scanAccountQuotaWindow(row)
}

func (r *accountQuotaWindowRepository) UpsertOpenRefresh(ctx context.Context, accountID int64, kind string, endAt time.Time, used *float64, windowMinutes *int) error {
	// Refresh only end/used on the open window. start_at is NOT moved: it is the
	// real cycle boundary (set at observe/reset time); sliding it with end would
	// corrupt the anchor used by history projection and later close logic.
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_quota_windows
		SET end_at = $3,
		    window_minutes = COALESCE($4, window_minutes),
		    used_percent_open = COALESCE($5, used_percent_open),
		    updated_at = NOW()
		WHERE account_id = $1 AND kind = $2 AND is_open = TRUE`,
		accountID, kind, endAt, windowMinutes, used,
	)
	return err
}

func (r *accountQuotaWindowRepository) CloseOpen(ctx context.Context, accountID int64, kind string, endAt time.Time, reason string, used *float64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_quota_windows
		SET is_open = FALSE,
		    end_at = $3,
		    closed_reason = $4,
		    used_percent_close = COALESCE($5, used_percent_close),
		    updated_at = NOW()
		WHERE account_id = $1 AND kind = $2 AND is_open = TRUE`,
		accountID, kind, endAt, reason, used,
	)
	return err
}

func (r *accountQuotaWindowRepository) CloseAndOpen(
	ctx context.Context,
	closeID int64,
	closeEnd time.Time,
	closeReason string,
	closeUsed *float64,
	open *service.AccountQuotaWindow,
) (*service.AccountQuotaWindow, error) {
	if open == nil {
		return nil, fmt.Errorf("nil open window")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if closeID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE account_quota_windows
			SET is_open = FALSE,
			    end_at = GREATEST(start_at + interval '1 second', $2::timestamptz),
			    closed_reason = $3,
			    used_percent_close = COALESCE($4, used_percent_close),
			    updated_at = NOW()
			WHERE id = $1 AND is_open = TRUE`,
			closeID, closeEnd, closeReason, closeUsed,
		); err != nil {
			return nil, err
		}
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO account_quota_windows (
			account_id, platform, kind, start_at, end_at, window_minutes, source,
			used_percent_open, is_open, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,TRUE,NOW(),NOW())
		RETURNING id, account_id, platform, kind, start_at, end_at, window_minutes, source,
		          COALESCE(closed_reason,''), used_percent_open, used_percent_close, is_open, created_at, updated_at`,
		open.AccountID, open.Platform, open.Kind, open.StartAt, open.EndAt, open.WindowMinutes,
		nullIfEmpty(open.Source, service.QuotaWindowSourceObserved), open.UsedPercentOpen,
	)
	out, err := scanAccountQuotaWindow(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *accountQuotaWindowRepository) InsertObservation(ctx context.Context, observation *service.AccountQuotaUsageObservation) (bool, error) {
	if observation == nil {
		return false, fmt.Errorf("nil observation")
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO account_quota_usage_observations (
			quota_window_id, account_id, platform, kind, observed_at, used_percent,
			requests, tokens, account_cost, standard_cost, user_cost
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (quota_window_id, used_percent) DO NOTHING`,
		observation.QuotaWindowID, observation.AccountID, observation.Platform, observation.Kind,
		observation.ObservedAt, observation.UsedPercent, observation.Requests, observation.Tokens,
		observation.AccountCost, observation.StandardCost, observation.UserCost,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *accountQuotaWindowRepository) HasObservation(ctx context.Context, quotaWindowID int64, usedPercent float64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_quota_usage_observations
			WHERE quota_window_id = $1 AND used_percent = $2
		)`, quotaWindowID, usedPercent).Scan(&exists)
	return exists, err
}

func (r *accountQuotaWindowRepository) ListObservations(ctx context.Context, quotaWindowID int64, limit int) ([]*service.AccountQuotaUsageObservation, error) {
	if limit <= 0 || limit > 200 {
		limit = 120
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, quota_window_id, account_id, platform, kind, observed_at, used_percent,
		       requests, tokens, account_cost, standard_cost, user_cost, created_at
		FROM (
			SELECT id, quota_window_id, account_id, platform, kind, observed_at, used_percent,
			       requests, tokens, account_cost, standard_cost, user_cost, created_at
			FROM account_quota_usage_observations
			WHERE quota_window_id = $1
			ORDER BY observed_at DESC
			LIMIT $2
		) recent
		ORDER BY observed_at ASC`, quotaWindowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*service.AccountQuotaUsageObservation, 0)
	for rows.Next() {
		observation := &service.AccountQuotaUsageObservation{}
		if err := rows.Scan(
			&observation.ID, &observation.QuotaWindowID, &observation.AccountID,
			&observation.Platform, &observation.Kind, &observation.ObservedAt,
			&observation.UsedPercent, &observation.Requests, &observation.Tokens,
			&observation.AccountCost, &observation.StandardCost, &observation.UserCost,
			&observation.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, observation)
	}
	return result, rows.Err()
}

type aqScan interface {
	Scan(dest ...any) error
}

func scanAccountQuotaWindow(row aqScan) (*service.AccountQuotaWindow, error) {
	var w service.AccountQuotaWindow
	var closed string
	var windowMinutes sql.NullInt64
	var usedOpen, usedClose sql.NullFloat64
	err := row.Scan(
		&w.ID, &w.AccountID, &w.Platform, &w.Kind, &w.StartAt, &w.EndAt, &windowMinutes, &w.Source,
		&closed, &usedOpen, &usedClose, &w.IsOpen, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	w.ClosedReason = closed
	if windowMinutes.Valid {
		v := int(windowMinutes.Int64)
		w.WindowMinutes = &v
	}
	if usedOpen.Valid {
		v := usedOpen.Float64
		w.UsedPercentOpen = &v
	}
	if usedClose.Valid {
		v := usedClose.Float64
		w.UsedPercentClose = &v
	}
	return &w, nil
}

func scanAccountQuotaWindows(rows *sql.Rows) ([]*service.AccountQuotaWindow, error) {
	var out []*service.AccountQuotaWindow
	for rows.Next() {
		w, err := scanAccountQuotaWindow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func nullIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
