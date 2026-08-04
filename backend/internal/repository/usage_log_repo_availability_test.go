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

func TestGetTrafficAvailabilityExcludesBusinessLimitedFailures(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{db: db}
	start := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta("AND NOT COALESCE(e.is_business_limited, false)")).
		WithArgs(start, end, "600 seconds").
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "success_count", "failure_count", "average_latency"}))

	result, err := repo.GetTrafficAvailability(context.Background(), start, end, 10*time.Minute, nil, "")

	require.NoError(t, err)
	require.Zero(t, result.FailureCount)
	require.Zero(t, result.SampleCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
