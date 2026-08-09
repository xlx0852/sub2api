//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestUpsertOpenRefresh_SyncsStartAt 验证修复：end_at 更新时 start_at 必须同步平移
// （start_at = end_at − window_minutes），避免上游 reset_at 漂移导致窗口长度失真。
func TestUpsertOpenRefresh_SyncsStartAt(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &accountQuotaWindowRepository{db: db}

	endAt := time.Date(2026, 8, 16, 9, 41, 26, 0, time.UTC)
	mins := 10080
	used := 0.0

	// SQL 必须同时更新 start_at（end_at - window_minutes）与 end_at/window_minutes/used。
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE account_quota_windows
		SET end_at = $3,
		    window_minutes = COALESCE($4, window_minutes),
		    start_at = $3 - (COALESCE($4, window_minutes) * INTERVAL '1 minute'),
		    used_percent_open = COALESCE($5, used_percent_open),
		    updated_at = NOW()
		WHERE account_id = $1 AND kind = $2 AND is_open = TRUE`)).
		WithArgs(int64(69), "7d", endAt, mins, used).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpsertOpenRefresh(context.Background(), 69, "7d", endAt, &used, &mins)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
