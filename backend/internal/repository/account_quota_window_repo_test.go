//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestUpsertOpenRefresh_KeepsRealStartAt verifies that same-window countdown
// calibration never rewrites the real opening boundary.
func TestUpsertOpenRefresh_KeepsRealStartAt(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &accountQuotaWindowRepository{db: db}

	endAt := time.Date(2026, 8, 16, 9, 41, 26, 0, time.UTC)
	mins := 10080
	used := 0.0

	// Live fields are refreshed while start_at remains the original boundary.
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE account_quota_windows
		SET end_at = $3,
		    window_minutes = COALESCE($4, window_minutes),
		    used_percent_open = COALESCE($5, used_percent_open),
		    updated_at = NOW()
		WHERE account_id = $1 AND kind = $2 AND is_open = TRUE`)).
		WithArgs(int64(69), "7d", endAt, mins, used).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertOpenRefresh(context.Background(), 69, "7d", endAt, &used, &mins)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertObservation_IsIdempotentByWindowAndPercent(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &accountQuotaWindowRepository{db: db}
	observedAt := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	observation := &service.AccountQuotaUsageObservation{
		QuotaWindowID: 1, AccountID: 69, Platform: "openai", Kind: "7d",
		ObservedAt: observedAt, UsedPercent: 10, Requests: 100, Tokens: 1000,
		AccountCost: 20, StandardCost: 20, UserCost: 2,
	}
	mock.ExpectExec("INSERT INTO account_quota_usage_observations").
		WithArgs(int64(1), int64(69), "openai", "7d", observedAt, float64(10), int64(100), int64(1000), float64(20), float64(20), float64(2)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	inserted, err := repo.InsertObservation(context.Background(), observation)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}
