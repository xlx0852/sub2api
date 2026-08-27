package repository

import (
	"os"
	"strings"
	"testing"
)

func TestFirstOutputAggregateBreakdownCombinesSumsAndCoverage(t *testing.T) {
	fullHour := firstOutputAggregate{
		eligible: 8,
		pre:      firstOutputStageAggregate{sumMs: 600, count: 6},
		upstream: firstOutputStageAggregate{sumMs: 4800, count: 6},
		total:    firstOutputStageAggregate{sumMs: 6000, count: 6},
	}
	rawTail := firstOutputAggregate{
		eligible: 2,
		pre:      firstOutputStageAggregate{sumMs: 200, count: 2},
		upstream: firstOutputStageAggregate{sumMs: 2000, count: 2},
		total:    firstOutputStageAggregate{sumMs: 2400, count: 2},
	}

	fullHour.add(rawTail)
	got := fullHour.breakdown()

	if got.EligibleStreamingRequests != 10 {
		t.Fatalf("eligible = %d, want 10", got.EligibleStreamingRequests)
	}
	if got.GatewayPreUpstream.AvgMs == nil || *got.GatewayPreUpstream.AvgMs != 100 {
		t.Fatalf("pre-upstream average = %v, want 100", got.GatewayPreUpstream.AvgMs)
	}
	if got.UpstreamFirstOutput.AvgMs == nil || *got.UpstreamFirstOutput.AvgMs != 850 {
		t.Fatalf("upstream average = %v, want 850", got.UpstreamFirstOutput.AvgMs)
	}
	if got.GatewayFirstOutputTotal.AvgMs == nil || *got.GatewayFirstOutputTotal.AvgMs != 1050 {
		t.Fatalf("total average = %v, want 1050", got.GatewayFirstOutputTotal.AvgMs)
	}
	if got.GatewayFirstOutputTotal.Coverage != 0.8 {
		t.Fatalf("coverage = %v, want 0.8", got.GatewayFirstOutputTotal.Coverage)
	}
	if got.UpstreamConnect.AvgMs != nil || got.UpstreamConnect.Coverage != 0 {
		t.Fatalf("missing stage should stay null with zero coverage: %+v", got.UpstreamConnect)
	}
}

func TestOpsTTFTStageMigrationContainsRawAndHourlyFields(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/202_ops_ttft_stage_breakdown.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	required := []string{
		"first_output_stages JSONB",
		"first_output_eligible_count BIGINT",
		"gateway_pre_upstream_sum_ms BIGINT",
		"upstream_connect_sum_ms BIGINT",
		"upstream_first_output_sum_ms BIGINT",
		"retry_failover_sum_ms BIGINT",
		"gateway_first_output_write_sum_ms BIGINT",
		"gateway_first_output_total_sum_ms BIGINT",
	}
	for _, field := range required {
		if !strings.Contains(sql, field) {
			t.Errorf("migration missing %q", field)
		}
	}
}
