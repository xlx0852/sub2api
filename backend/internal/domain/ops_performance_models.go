package domain

import "time"

type OpsPerformanceFilter struct {
	StartTime   time.Time
	EndTime     time.Time
	Platform    string
	Model       string
	GroupID     *int64
	RequestType *int16
	PointLimit  int
}

type OpsPerformanceMetric struct {
	P50         *float64 `json:"p50"`
	P90         *float64 `json:"p90"`
	Unit        string   `json:"unit"`
	SampleCount int64    `json:"sample_count"`
	Coverage    float64  `json:"coverage"`
}

type OpsPerformanceSummary struct {
	RequestCount  int64                `json:"request_count"`
	SuccessCount  int64                `json:"success_count"`
	FailureCount  int64                `json:"failure_count"`
	SuccessRate   *float64             `json:"success_rate"`
	TTFT          OpsPerformanceMetric `json:"ttft"`
	GenerationTPS OpsPerformanceMetric `json:"generation_tps"`
	Duration      OpsPerformanceMetric `json:"duration"`
	InputTokens   OpsPerformanceMetric `json:"input_tokens"`
}

type OpsPerformancePoint struct {
	At            time.Time `json:"at"`
	Model         string    `json:"model"`
	Success       bool      `json:"success"`
	StatusCode    *int      `json:"status_code,omitempty"`
	TTFTMs        *float64  `json:"ttft_ms,omitempty"`
	GenerationTPS *float64  `json:"generation_tps,omitempty"`
	DurationMs    *float64  `json:"duration_ms,omitempty"`
	InputTokens   *int64    `json:"input_tokens,omitempty"`
	FailureType   string    `json:"failure_type,omitempty"`
}

type OpsPerformanceWindow struct {
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Summary       OpsPerformanceSummary  `json:"summary"`
	Points        []*OpsPerformancePoint `json:"points"`
	PointLimit    int                    `json:"point_limit"`
	PointsSampled bool                   `json:"points_sampled"`
}

type OpsPerformanceDeltas struct {
	SuccessRate      *float64 `json:"success_rate"`
	TTFTP50          *float64 `json:"ttft_p50"`
	GenerationTPSP50 *float64 `json:"generation_tps_p50"`
	DurationP50      *float64 `json:"duration_p50"`
	InputTokensP50   *float64 `json:"input_tokens_p50"`
}

type OpsPerformanceResponse struct {
	GeneratedAt      time.Time             `json:"generated_at"`
	Timezone         string                `json:"timezone"`
	Window           *OpsPerformanceWindow `json:"window"`
	Comparison       *OpsPerformanceWindow `json:"comparison,omitempty"`
	ComparisonDeltas *OpsPerformanceDeltas `json:"comparison_deltas,omitempty"`
}
