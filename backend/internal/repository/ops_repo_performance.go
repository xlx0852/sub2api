package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func (r *opsRepository) GetPerformanceWindow(ctx context.Context, filter *domain.OpsPerformanceFilter) (*domain.OpsPerformanceWindow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	start, end := filter.StartTime.UTC(), filter.EndTime.UTC()
	usageFilter := &domain.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: filter.Platform, GroupID: filter.GroupID, RequestType: filter.RequestType}
	usageJoin, usageWhere, usageArgs, usageNext := buildUsageWhere(usageFilter, start, end, 1)
	usageWhere, usageArgs, usageNext = appendPerformanceModel(usageWhere, usageArgs, usageNext, "ul.model", filter.Model)

	errorFilter := &domain.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: filter.Platform, GroupID: filter.GroupID, RequestType: filter.RequestType}
	errorWhere, errorArgs, errorNext := buildErrorWhere(errorFilter, start, end, 1)
	errorWhere, errorArgs, errorNext = appendPerformanceModel(errorWhere, errorArgs, errorNext, "model", filter.Model)

	var successCount, failureCount int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs ul "+usageJoin+" "+usageWhere, usageArgs...).Scan(&successCount); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_error_logs "+errorWhere+" AND COALESCE(status_code, 0) >= 400", errorArgs...).Scan(&failureCount); err != nil {
		return nil, err
	}

	metricSQL := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL),
  COUNT(first_token_ms),
  percentile_cont(0.50) WITHIN GROUP (ORDER BY CASE WHEN output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0) THEN output_tokens * 1000.0 / (duration_ms - first_token_ms) END) FILTER (WHERE output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0)),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY CASE WHEN output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0) THEN output_tokens * 1000.0 / (duration_ms - first_token_ms) END) FILTER (WHERE output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0)),
  COUNT(*) FILTER (WHERE output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0)),
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL),
  COUNT(duration_ms),
  percentile_cont(0.50) WITHIN GROUP (ORDER BY input_tokens),
  percentile_cont(0.90) WITHIN GROUP (ORDER BY input_tokens),
  COUNT(input_tokens)
FROM usage_logs ul ` + usageJoin + ` ` + usageWhere
	var ttftP50, ttftP90, tpsP50, tpsP90, durationP50, durationP90, inputP50, inputP90 sql.NullFloat64
	var ttftCount, tpsCount, durationCount, inputCount int64
	if err := r.db.QueryRowContext(ctx, metricSQL, usageArgs...).Scan(&ttftP50, &ttftP90, &ttftCount, &tpsP50, &tpsP90, &tpsCount, &durationP50, &durationP90, &durationCount, &inputP50, &inputP90, &inputCount); err != nil {
		return nil, err
	}

	total := successCount + failureCount
	window := &domain.OpsPerformanceWindow{
		StartTime: start, EndTime: end, PointLimit: filter.PointLimit,
		Summary: domain.OpsPerformanceSummary{RequestCount: total, SuccessCount: successCount, FailureCount: failureCount},
	}
	if total > 0 {
		rate := float64(successCount) / float64(total)
		window.Summary.SuccessRate = &rate
	}
	window.Summary.TTFT = performanceMetric(ttftP50, ttftP90, ttftCount, total, "ms")
	window.Summary.GenerationTPS = performanceMetric(tpsP50, tpsP90, tpsCount, total, "tok/s")
	window.Summary.Duration = performanceMetric(durationP50, durationP90, durationCount, total, "ms")
	window.Summary.InputTokens = performanceMetric(inputP50, inputP90, inputCount, total, "tok")

	points, sampled, err := r.queryPerformancePoints(ctx, filter, usageJoin, usageWhere, usageArgs, errorWhere, errorArgs, usageNext, errorNext)
	if err != nil {
		return nil, err
	}
	window.Points, window.PointsSampled = points, sampled
	return window, nil
}

func performanceMetric(p50, p90 sql.NullFloat64, count, total int64, unit string) domain.OpsPerformanceMetric {
	metric := domain.OpsPerformanceMetric{SampleCount: count, Unit: unit}
	if p50.Valid {
		v := p50.Float64
		metric.P50 = &v
	}
	if p90.Valid {
		v := p90.Float64
		metric.P90 = &v
	}
	if total > 0 {
		metric.Coverage = float64(count) / float64(total)
	}
	return metric
}

func appendPerformanceModel(where string, args []any, next int, column, model string) (string, []any, int) {
	model = strings.TrimSpace(model)
	if model == "" {
		return where, args, next
	}
	args = append(args, model)
	return where + fmt.Sprintf(" AND %s = $%d", column, next), args, next + 1
}

func (r *opsRepository) queryPerformancePoints(ctx context.Context, filter *domain.OpsPerformanceFilter, usageJoin, usageWhere string, usageArgs []any, errorWhere string, errorArgs []any, usageNext, errorNext int) ([]*domain.OpsPerformancePoint, bool, error) {
	limit := filter.PointLimit
	if limit <= 0 {
		limit = 500
	}
	// Query both sources independently so usage and error schemas remain decoupled.
	usageSQL := `SELECT ul.created_at, COALESCE(ul.model, ''), ul.first_token_ms, ul.output_tokens, ul.duration_ms, ul.input_tokens FROM usage_logs ul ` + usageJoin + ` ` + usageWhere + ` ORDER BY ul.created_at DESC LIMIT $` + fmt.Sprint(usageNext)
	usageQueryArgs := append(append([]any(nil), usageArgs...), limit+1)
	rows, err := r.db.QueryContext(ctx, usageSQL, usageQueryArgs...)
	if err != nil {
		return nil, false, err
	}
	points := make([]*domain.OpsPerformancePoint, 0, limit)
	for rows.Next() {
		var at time.Time
		var model sql.NullString
		var first, duration sql.NullFloat64
		var output, input sql.NullInt64
		if err := rows.Scan(&at, &model, &first, &output, &duration, &input); err != nil {
			rows.Close()
			return nil, false, err
		}
		p := &domain.OpsPerformancePoint{At: at, Model: nullableSQLString(model), Success: true}
		if first.Valid {
			v := first.Float64
			p.TTFTMs = &v
		}
		if duration.Valid {
			v := duration.Float64
			p.DurationMs = &v
		}
		if input.Valid {
			v := input.Int64
			p.InputTokens = &v
		}
		if output.Valid && output.Int64 > 0 && duration.Valid && duration.Float64 > func() float64 {
			if first.Valid {
				return first.Float64
			}
			return 0
		}() {
			firstValue := float64(0)
			if first.Valid {
				firstValue = first.Float64
			}
			v := float64(output.Int64) * 1000 / (duration.Float64 - firstValue)
			p.GenerationTPS = &v
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, err
	}
	rows.Close()

	errorSQL := `SELECT created_at, COALESCE(model, ''), COALESCE(status_code, upstream_status_code), COALESCE(error_type, '') FROM ops_error_logs ` + errorWhere + ` AND COALESCE(status_code, 0) >= 400 ORDER BY created_at DESC LIMIT $` + fmt.Sprint(errorNext)
	errorQueryArgs := append(append([]any(nil), errorArgs...), limit+1)
	errorRows, err := r.db.QueryContext(ctx, errorSQL, errorQueryArgs...)
	if err != nil {
		return nil, false, err
	}
	for errorRows.Next() {
		var at time.Time
		var model, failure sql.NullString
		var status sql.NullInt64
		if err := errorRows.Scan(&at, &model, &status, &failure); err != nil {
			errorRows.Close()
			return nil, false, err
		}
		p := &domain.OpsPerformancePoint{At: at, Model: nullableSQLString(model), Success: false, FailureType: nullableSQLString(failure)}
		if status.Valid {
			v := int(status.Int64)
			p.StatusCode = &v
		}
		points = append(points, p)
	}
	if err := errorRows.Err(); err != nil {
		errorRows.Close()
		return nil, false, err
	}
	errorRows.Close()
	// Merge the independently capped streams and keep the newest points globally.
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j].At.After(points[j-1].At); j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
	sampled := len(points) > limit
	if len(points) > limit {
		points = points[:limit]
	}
	return points, sampled, nil
}

func nullableSQLString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
