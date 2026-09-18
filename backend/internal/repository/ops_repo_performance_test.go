package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestPerformanceMetricKeepsMissingValuesNullAndReportsCoverage(t *testing.T) {
	metric := performanceMetric(sql.NullFloat64{Float64: 120, Valid: true}, sql.NullFloat64{}, 3, 10, "ms")
	require.NotNil(t, metric.P50)
	require.Equal(t, 120.0, *metric.P50)
	require.Nil(t, metric.P90)
	require.Equal(t, "ms", metric.Unit)
	require.Equal(t, int64(3), metric.SampleCount)
	require.InDelta(t, 0.3, metric.Coverage, 0.0001)
}

func TestAppendPerformanceModelUsesParameterizedFilter(t *testing.T) {
	where, args, next := appendPerformanceModel("WHERE created_at >= $1", []any{"start"}, 2, "ul.model", "gpt-5")
	require.Equal(t, "WHERE created_at >= $1 AND ul.model = $2", where)
	require.Equal(t, []any{"start", "gpt-5"}, args)
	require.Equal(t, 3, next)
}

func TestNullableSQLStringTreatsInvalidAsEmpty(t *testing.T) {
	require.Equal(t, "", nullableSQLString(sql.NullString{}))
	require.Equal(t, "gpt-5", nullableSQLString(sql.NullString{String: "gpt-5", Valid: true}))
}

func TestGetPerformanceWindowScansNullModelAsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	repo := &opsRepository{db: db}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM usage_logs ul`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_error_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`percentile_cont`).
		WillReturnRows(sqlmock.NewRows([]string{
			"ttft_p50", "ttft_p90", "ttft_count", "tps_p50", "tps_p90", "tps_count", "duration_p50", "duration_p90", "duration_count", "input_p50", "input_p90", "input_count",
		}).AddRow(nil, nil, 0, nil, nil, 0, nil, nil, 0, nil, nil, 0))
	mock.ExpectQuery(`SELECT ul.created_at, COALESCE\(ul.model, ''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "model", "first_token_ms", "output_tokens", "duration_ms", "input_tokens"}).
			AddRow(start.Add(time.Minute), nil, nil, nil, nil, nil))
	mock.ExpectQuery(`SELECT created_at, COALESCE\(model, ''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "model", "status", "error_type"}).
			AddRow(start.Add(2*time.Minute), nil, 500, nil))

	window, err := repo.GetPerformanceWindow(context.Background(), &domain.OpsPerformanceFilter{StartTime: start, EndTime: end, PointLimit: 5})
	require.NoError(t, err)
	require.Equal(t, int64(2), window.Summary.RequestCount)
	require.Len(t, window.Points, 2)
	require.Equal(t, "", window.Points[0].Model)
	require.Equal(t, "", window.Points[1].Model)
	require.NoError(t, mock.ExpectationsWereMet())
}
