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

func TestGetGroupTrafficAvailabilityFiltersByGroupAndBusinessLimited(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{db: db}
	start := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	end := start.Add(20 * time.Minute)
	groupID := int64(42)

	// Ensure the SQL carries the group filter and excludes business-limited failures.
	mock.ExpectQuery(regexp.QuoteMeta("AND e.group_id = $3")).
		WithArgs(start, end, groupID, "600 seconds").
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "success_count", "failure_count", "average_latency"}))

	result, err := repo.GetGroupTrafficAvailability(context.Background(), groupID, start, end, 10*time.Minute)

	require.NoError(t, err)
	require.Equal(t, groupID, result.GroupID)
	require.Equal(t, "no_traffic", result.Status)
	require.Zero(t, result.SampleCount)
	require.Len(t, result.Buckets, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupTrafficAvailabilityComputesRateAndLatency(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{db: db}
	start := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	groupID := int64(7)

	lat := 1500.0
	mock.ExpectQuery("date_bin").
		WithArgs(start, end, groupID, "600 seconds").
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "success_count", "failure_count", "average_latency"}).
			AddRow(start, int64(9), int64(1), lat))

	result, err := repo.GetGroupTrafficAvailability(context.Background(), groupID, start, end, 10*time.Minute)

	require.NoError(t, err)
	require.Equal(t, int64(9), result.SuccessCount)
	require.Equal(t, int64(1), result.FailureCount)
	require.Equal(t, int64(10), result.SampleCount)
	require.NotNil(t, result.SuccessRate)
	require.InDelta(t, 90.0, *result.SuccessRate, 1e-9)
	require.Equal(t, "attention", result.Status)
	require.NotNil(t, result.AverageLatencyMs)
	require.InDelta(t, 1500.0, *result.AverageLatencyMs, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupTrafficAvailabilityRejectsInvalidRange(t *testing.T) {
	repo := &usageLogRepository{}
	_, err := repo.GetGroupTrafficAvailability(context.Background(), 0, time.Now(), time.Now().Add(time.Hour), time.Minute)
	require.Error(t, err)
}
