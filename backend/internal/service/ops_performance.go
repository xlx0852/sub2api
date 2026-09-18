package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetPerformanceWindow(ctx context.Context, filter *OpsPerformanceFilter) (*OpsPerformanceWindow, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}
	if filter.PointLimit <= 0 {
		filter.PointLimit = 500
	}
	if filter.PointLimit > 2000 {
		return nil, infraerrors.BadRequest("OPS_POINT_LIMIT_INVALID", "point_limit must be between 1 and 2000")
	}
	return s.opsRepo.GetPerformanceWindow(ctx, filter)
}

func (s *OpsService) GetPerformanceAnalytics(ctx context.Context, filter *OpsPerformanceFilter, comparison *OpsPerformanceFilter, timezone string) (*OpsPerformanceResponse, error) {
	window, err := s.GetPerformanceWindow(ctx, filter)
	if err != nil {
		return nil, err
	}
	var previous *OpsPerformanceWindow
	var deltas *OpsPerformanceDeltas
	if comparison != nil {
		previous, err = s.GetPerformanceWindow(ctx, comparison)
		if err != nil {
			return nil, err
		}
		deltas = performanceDeltas(window, previous)
	}
	return &OpsPerformanceResponse{GeneratedAt: time.Now().UTC(), Timezone: timezone, Window: window, Comparison: previous, ComparisonDeltas: deltas}, nil
}

func performanceDeltas(current, previous *OpsPerformanceWindow) *OpsPerformanceDeltas {
	if current == nil || previous == nil {
		return nil
	}
	return &OpsPerformanceDeltas{
		SuccessRate:      relativeMetric(current.Summary.SuccessRate, previous.Summary.SuccessRate),
		TTFTP50:          relativeMetric(current.Summary.TTFT.P50, previous.Summary.TTFT.P50),
		GenerationTPSP50: relativeMetric(current.Summary.GenerationTPS.P50, previous.Summary.GenerationTPS.P50),
		DurationP50:      relativeMetric(current.Summary.Duration.P50, previous.Summary.Duration.P50),
		InputTokensP50:   relativeMetric(current.Summary.InputTokens.P50, previous.Summary.InputTokens.P50),
	}
}

func relativeMetric(current, previous *float64) *float64 {
	if current == nil || previous == nil || *previous == 0 {
		return nil
	}
	value := (*current / *previous) - 1
	return &value
}
