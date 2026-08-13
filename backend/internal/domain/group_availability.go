// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

// GroupTrafficAvailability is real-traffic availability for a group.
// Success = usage_logs rows (per request_id); failure = ops_error_logs rows
// (status >= 400, excluding business-limited). Buckets use the same 10m grid
// as traffic_availability for the 24h view.
type GroupTrafficAvailability struct {
	GroupID          int64                       `json:"group_id"`
	StartAt          time.Time                   `json:"start_at"`
	EndAt            time.Time                   `json:"end_at"`
	BucketMinutes    int                         `json:"bucket_minutes"`
	SuccessCount     int64                       `json:"success_count"`
	FailureCount     int64                       `json:"failure_count"`
	SampleCount      int64                       `json:"sample_count"`
	SuccessRate      *float64                    `json:"success_rate"`
	AverageLatencyMs *float64                    `json:"average_latency_ms"`
	Status           string                      `json:"status"`
	Buckets          []TrafficAvailabilityBucket `json:"buckets,omitempty"`
}
