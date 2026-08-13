// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

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
