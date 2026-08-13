package service

import (
	"context"
	"fmt"
	"time"
)

const trafficAvailabilityBucketSize = 10 * time.Minute

func trafficAvailabilityStatus(samples int64, rate float64) string {
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

func (s *UsageService) GetTrafficAvailability(ctx context.Context, userID *int64, platform string) (*TrafficAvailability, error) {
	end := time.Now().UTC().Truncate(trafficAvailabilityBucketSize).Add(trafficAvailabilityBucketSize)
	start := end.Add(-24 * time.Hour)
	result, err := s.usageRepo.GetTrafficAvailability(ctx, start, end, trafficAvailabilityBucketSize, userID, platform)
	if err != nil {
		return nil, fmt.Errorf("get traffic availability: %w", err)
	}
	return result, nil
}
