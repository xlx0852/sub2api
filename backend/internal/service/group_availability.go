package service

import (
	"context"
	"fmt"
	"strconv"
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
// 30s 进程内缓存: 24h 路径是 request_id 级 DISTINCT ON + date_bin 分桶全扫,
// 用户开 dashboard 时每个 group 各触发一次(还有管理端单 group 查询);
// key=groupID(group 级数据, 与用户无关), 30s 内同 group 只回源一次。
func (s *UsageService) GetGroupTrafficAvailability(ctx context.Context, groupID int64) (*GroupTrafficAvailabilitySummary, error) {
	if groupID <= 0 {
		return nil, fmt.Errorf("invalid group id")
	}
	if s.groupTrafficAvailabilityCache == nil {
		return s.fetchGroupTrafficAvailability(ctx, groupID)
	}
	key := strconv.FormatInt(groupID, 10)
	return s.groupTrafficAvailabilityCache.Load(ctx, key, func(ctx context.Context) (*GroupTrafficAvailabilitySummary, error) {
		return s.fetchGroupTrafficAvailability(ctx, groupID)
	})
}

func (s *UsageService) fetchGroupTrafficAvailability(ctx context.Context, groupID int64) (*GroupTrafficAvailabilitySummary, error) {
	now := time.Now().UTC()
	end := now.Truncate(groupAvailabilityBucket).Add(groupAvailabilityBucket)

	day, err := s.usageRepo.GetGroupTrafficAvailability(ctx, groupID, end.Add(-24*time.Hour), end, groupAvailabilityBucket)
	if err != nil {
		return nil, fmt.Errorf("get group 24h availability: %w", err)
	}
	// 7d summary: cheap aggregate without request_id dedup / buckets.
	week, err := s.usageRepo.GetGroupTrafficAvailabilityRollup(ctx, groupID, end.Add(-7*24*time.Hour), end)
	if err != nil {
		return nil, fmt.Errorf("get group 7d availability: %w", err)
	}

	return &GroupTrafficAvailabilitySummary{GroupID: groupID, Day: day, Week: week}, nil
}
