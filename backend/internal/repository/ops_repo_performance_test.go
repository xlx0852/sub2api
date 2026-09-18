package repository

import (
	"database/sql"
	"testing"

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
