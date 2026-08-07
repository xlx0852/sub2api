package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GetGroupAvailability aggregates real-traffic availability for a group:
// successes from usage_logs (per request_id), failures from ops_error_logs
// (status >= 400, excluding business-limited rows), aligned with traffic_availability.
func (r *usageLogRepository) GetGroupTrafficAvailability(ctx context.Context, groupID int64, start, end time.Time, bucketSize time.Duration) (*service.GroupTrafficAvailability, error) {
	if groupID <= 0 || bucketSize <= 0 || !start.Before(end) {
		return nil, fmt.Errorf("invalid group availability range")
	}
	args := []any{start.UTC(), end.UTC(), groupID, fmt.Sprintf("%d seconds", int64(bucketSize.Seconds()))}
	query := `
WITH successes AS (
  SELECT DISTINCT ON (ul.request_id)
    ul.request_id, ul.created_at, ul.duration_ms
  FROM usage_logs ul
  WHERE ul.created_at >= $1 AND ul.created_at < $2
    AND ul.group_id = $3
  ORDER BY ul.request_id, ul.created_at DESC
), failures AS (
  SELECT DISTINCT ON (CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END)
    CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END AS event_key,
    e.request_id, e.created_at
  FROM ops_error_logs e
  WHERE e.created_at >= $1 AND e.created_at < $2
    AND e.group_id = $3
    AND COALESCE(e.status_code, 0) >= 400
    AND NOT COALESCE(e.is_business_limited, false)
    AND (COALESCE(e.request_id, '') = '' OR NOT EXISTS (
      SELECT 1 FROM successes s WHERE s.request_id = e.request_id
    ))
  ORDER BY (CASE WHEN COALESCE(e.request_id, '') = '' THEN 'error:' || e.id::text ELSE e.request_id END), e.created_at DESC
), events AS (
  SELECT date_bin($4::interval, created_at, TIMESTAMPTZ '1970-01-01') AS bucket,
         1::bigint AS success_count, 0::bigint AS failure_count, duration_ms::double precision AS duration_ms
  FROM successes
  UNION ALL
  SELECT date_bin($4::interval, created_at, TIMESTAMPTZ '1970-01-01') AS bucket,
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

	result := &service.GroupTrafficAvailability{
		GroupID:       groupID,
		StartAt:       start.UTC(),
		EndAt:         end.UTC(),
		BucketMinutes: int(bucketSize / time.Minute),
		Buckets:       make([]service.TrafficAvailabilityBucket, 0, int(end.Sub(start)/bucketSize)),
	}
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
		result.Status = trafficStatus(result.SampleCount, rate)
	} else {
		result.Status = "no_traffic"
	}
	if latencySamples > 0 {
		avg := latencyWeighted / float64(latencySamples)
		result.AverageLatencyMs = &avg
	}
	return result, nil
}

// GetGroupTrafficAvailabilityRollup returns a single-window aggregate without
// request_id-level dedup and without time buckets. Used for the 7d summary so
// long windows don't pay for a huge DISTINCT ON sort.
func (r *usageLogRepository) GetGroupTrafficAvailabilityRollup(ctx context.Context, groupID int64, start, end time.Time) (*service.GroupTrafficAvailability, error) {
	if groupID <= 0 || !start.Before(end) {
		return nil, fmt.Errorf("invalid group availability range")
	}
	query := `
SELECT
  (SELECT COUNT(*) FROM usage_logs ul
     WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.group_id = $3) AS success_count,
  (SELECT COUNT(*) FROM ops_error_logs e
     WHERE e.created_at >= $1 AND e.created_at < $2 AND e.group_id = $3
       AND COALESCE(e.status_code, 0) >= 400
       AND NOT COALESCE(e.is_business_limited, false)) AS failure_count,
  (SELECT AVG(ul.duration_ms) FROM usage_logs ul
     WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.group_id = $3) AS avg_latency
`
	var success, failure int64
	var latency *float64
	if err := r.db.QueryRowContext(ctx, query, start.UTC(), end.UTC(), groupID).
		Scan(&success, &failure, &latency); err != nil {
		return nil, err
	}
	result := &service.GroupTrafficAvailability{
		GroupID:      groupID,
		StartAt:      start.UTC(),
		EndAt:        end.UTC(),
		SuccessCount: success,
		FailureCount: failure,
	}
	result.SampleCount = success + failure
	if result.SampleCount > 0 {
		rate := float64(success) * 100 / float64(result.SampleCount)
		result.SuccessRate = &rate
		result.Status = trafficStatus(result.SampleCount, rate)
	} else {
		result.Status = "no_traffic"
	}
	if latency != nil {
		avg := *latency
		result.AverageLatencyMs = &avg
	}
	return result, nil
}
