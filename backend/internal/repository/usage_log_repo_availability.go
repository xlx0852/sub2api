package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageLogRepository) GetTrafficAvailability(ctx context.Context, start, end time.Time, bucketSize time.Duration, userID *int64, platform string) (*service.TrafficAvailability, error) {
	if bucketSize <= 0 || !start.Before(end) {
		return nil, fmt.Errorf("invalid availability range")
	}
	userClauseUsage, userClauseError := "", ""
	args := []any{start.UTC(), end.UTC(), fmt.Sprintf("%d seconds", int64(bucketSize.Seconds()))}
	if userID != nil {
		args = append(args, *userID)
		index := len(args)
		userClauseUsage = fmt.Sprintf(" AND ul.user_id = $%d", index)
		userClauseError = fmt.Sprintf(" AND e.user_id = $%d", index)
	}
	platformClauseUsage, platformClauseError := "", ""
	if platform = strings.ToLower(strings.TrimSpace(platform)); platform != "" {
		if platform == "grok" {
			platformClauseUsage = " AND a.platform IN ('grok', 'xai')"
			platformClauseError = " AND LOWER(e.platform) IN ('grok', 'xai')"
		} else {
			args = append(args, platform)
			index := len(args)
			platformClauseUsage = fmt.Sprintf(" AND a.platform = $%d", index)
			platformClauseError = fmt.Sprintf(" AND LOWER(e.platform) = $%d", index)
		}
	}
	query := `
WITH successes AS (
  SELECT DISTINCT ON (ul.request_id)
    ul.request_id, ul.created_at, ul.duration_ms
  FROM usage_logs ul
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2` + userClauseUsage + platformClauseUsage + `
  ORDER BY ul.request_id, ul.created_at DESC
), failures AS (
  SELECT DISTINCT ON (CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END)
    CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END AS event_key,
    e.request_id, e.created_at
  FROM ops_error_logs e
  WHERE e.created_at >= $1 AND e.created_at < $2
    AND COALESCE(e.status_code, 0) >= 400` + userClauseError + platformClauseError + `
    AND NOT COALESCE(e.is_business_limited, false)
    AND (COALESCE(e.request_id, '') = '' OR NOT EXISTS (
      SELECT 1 FROM successes s WHERE s.request_id = e.request_id
    ))
  ORDER BY (CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END), e.created_at DESC
), events AS (
  SELECT date_bin($3::interval, created_at, TIMESTAMPTZ '1970-01-01') AS bucket,
         1::bigint AS success_count, 0::bigint AS failure_count, duration_ms::double precision AS duration_ms
  FROM successes
  UNION ALL
  SELECT date_bin($3::interval, created_at, TIMESTAMPTZ '1970-01-01') AS bucket,
         0::bigint, 1::bigint, NULL::double precision
  FROM failures
)
SELECT bucket, SUM(success_count), SUM(failure_count), AVG(duration_ms)
FROM events GROUP BY bucket ORDER BY bucket`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type aggregate struct {
		success, failure int64
		latency          *float64
	}
	byBucket := make(map[time.Time]aggregate)
	for rows.Next() {
		var bucket time.Time
		var a aggregate
		if err := rows.Scan(&bucket, &a.success, &a.failure, &a.latency); err != nil {
			return nil, err
		}
		byBucket[bucket.UTC()] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &service.TrafficAvailability{StartAt: start.UTC(), EndAt: end.UTC(), BucketMinutes: int(bucketSize / time.Minute), Buckets: make([]service.TrafficAvailabilityBucket, 0, int(end.Sub(start)/bucketSize))}
	var latencyWeighted float64
	var latencySamples int64
	for cursor := start.UTC(); cursor.Before(end.UTC()); cursor = cursor.Add(bucketSize) {
		a := byBucket[cursor]
		samples := a.success + a.failure
		bucket := service.TrafficAvailabilityBucket{StartAt: cursor, SuccessCount: a.success, FailureCount: a.failure, SampleCount: samples, AverageLatency: a.latency}
		if samples > 0 {
			rate := float64(a.success) * 100 / float64(samples)
			bucket.SuccessRate = &rate
			bucket.Status = trafficStatus(samples, rate)
		} else {
			bucket.Status = "no_traffic"
		}
		result.Buckets = append(result.Buckets, bucket)
		result.SuccessCount += a.success
		result.FailureCount += a.failure
		if a.latency != nil {
			latencyWeighted += *a.latency * float64(a.success)
			latencySamples += a.success
		}
	}
	result.SampleCount = result.SuccessCount + result.FailureCount
	if result.SampleCount > 0 {
		rate := float64(result.SuccessCount) * 100 / float64(result.SampleCount)
		result.SuccessRate = &rate
	}
	if latencySamples > 0 {
		avg := latencyWeighted / float64(latencySamples)
		result.AverageLatencyMs = &avg
	}
	return result, nil
}

func trafficStatus(samples int64, rate float64) string {
	if samples == 0 {
		return "no_traffic"
	}
	if rate >= 99 {
		return "healthy"
	}
	if rate >= 95 {
		return "degraded"
	}
	return "attention"
}
