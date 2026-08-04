package service

import "testing"

func TestTrafficAvailabilityStatusThresholds(t *testing.T) {
	tests := []struct {
		samples int64
		rate    float64
		want    string
	}{
		{0, 100, "no_traffic"}, {10, 99, "healthy"}, {10, 98.99, "degraded"}, {10, 95, "degraded"}, {10, 94.99, "attention"},
	}
	for _, tt := range tests {
		if got := trafficAvailabilityStatus(tt.samples, tt.rate); got != tt.want {
			t.Fatalf("samples=%d rate=%v: got %s want %s", tt.samples, tt.rate, got, tt.want)
		}
	}
}
