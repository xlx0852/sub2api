package service

import (
	"context"
	"fmt"
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

// GroupAvailabilitySummary bundles 24h/7d rollups with the 24h bucket series.
type GroupTrafficAvailabilitySummary struct {
	GroupID int64                     `json:"group_id"`
	Day     *GroupTrafficAvailability `json:"day"`
	Week    *GroupTrafficAvailability `json:"week"`
}

const groupAvailabilityBucket = 10 * time.Minute

// GetGroupAvailability returns real-traffic availability for a group.
func (s *UsageService) GetGroupTrafficAvailability(ctx context.Context, groupID int64) (*GroupTrafficAvailabilitySummary, error) {
	if groupID <= 0 {
		return nil, fmt.Errorf("invalid group id")
	}
	now := time.Now().UTC()
	end := now.Truncate(groupAvailabilityBucket).Add(groupAvailabilityBucket)

	day, err := s.usageRepo.GetGroupTrafficAvailability(ctx, groupID, end.Add(-24*time.Hour), end, groupAvailabilityBucket)
	if err != nil {
		return nil, fmt.Errorf("get group 24h availability: %w", err)
	}
	week, err := s.usageRepo.GetGroupTrafficAvailability(ctx, groupID, end.Add(-7*24*time.Hour), end, 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("get group 7d availability: %w", err)
	}
	// 7d is a single rollup — drop bucket series to keep payload small.
	week.Buckets = nil

	return &GroupTrafficAvailabilitySummary{GroupID: groupID, Day: day, Week: week}, nil
}
