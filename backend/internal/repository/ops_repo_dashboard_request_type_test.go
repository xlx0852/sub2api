package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"strings"
	"testing"
)

func TestResolveOpsDashboardQueryMode_RequestTypeForcesRaw(t *testing.T) {
	requestType := int16(domain.RequestTypeWSV2)
	filter := &domain.OpsDashboardFilter{
		QueryMode:   domain.OpsQueryModePreagg,
		RequestType: &requestType,
	}

	if got := resolveOpsDashboardQueryMode(filter); got != domain.OpsQueryModeRaw {
		t.Fatalf("resolveOpsDashboardQueryMode() = %s, want %s", got, domain.OpsQueryModeRaw)
	}
}

func TestBuildUsageWhere_RequestTypeUsesLegacyCompatibleCondition(t *testing.T) {
	requestType := int16(domain.RequestTypeWSV2)
	filter := &domain.OpsDashboardFilter{RequestType: &requestType}

	_, where, args, _ := buildUsageWhere(filter, testTimeA, testTimeB, 1)
	if !strings.Contains(where, "ul.request_type") {
		t.Fatalf("where should include request_type condition: %s", where)
	}
	if !strings.Contains(where, "ul.openai_ws_mode = TRUE") {
		t.Fatalf("where should include openai_ws_mode compatibility: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
}

func TestBuildErrorWhere_RequestTypeUsesQualifiedCondition(t *testing.T) {
	requestType := int16(domain.RequestTypeStream)
	filter := &domain.OpsDashboardFilter{RequestType: &requestType}

	where, args, _ := buildErrorWhere(filter, testTimeA, testTimeB, 1)
	if !strings.Contains(where, "request_type") {
		t.Fatalf("where should include request_type condition: %s", where)
	}
	if !strings.Contains(where, "stream = TRUE") {
		t.Fatalf("where should include stream compatibility: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
}
