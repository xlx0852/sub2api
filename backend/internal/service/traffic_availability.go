package service

import (
	"context"
	"fmt"
	"time"
)

const trafficAvailabilityBucketSize = 10 * time.Minute

type TrafficAvailabilityBucket struct {
	StartAt        time.Time `json:"start_at"`
	SuccessCount   int64     `json:"success_count"`
	FailureCount   int64     `json:"failure_count"`
	SampleCount    int64     `json:"sample_count"`
	SuccessRate    *float64  `json:"success_rate"`
	AverageLatency *float64  `json:"average_latency_ms"`
	Status         string    `json:"status"`
}

type TrafficAvailability struct {
	StartAt          time.Time                   `json:"start_at"`
	EndAt            time.Time                   `json:"end_at"`
	BucketMinutes    int                         `json:"bucket_minutes"`
	SuccessCount     int64                       `json:"success_count"`
	FailureCount     int64                       `json:"failure_count"`
	SampleCount      int64                       `json:"sample_count"`
	SuccessRate      *float64                    `json:"success_rate"`
	AverageLatencyMs *float64                    `json:"average_latency_ms"`
	Buckets          []TrafficAvailabilityBucket `json:"buckets"`
}

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
