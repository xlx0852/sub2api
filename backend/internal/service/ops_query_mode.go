package service

import (
	"errors"
	"strings"
)

func ParseOpsQueryMode(raw string) OpsQueryMode {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case string(OpsQueryModeRaw):
		return OpsQueryModeRaw
	case string(OpsQueryModePreagg):
		return OpsQueryModePreagg
	default:
		return OpsQueryModeAuto
	}
}

func shouldFallbackOpsPreagg(filter *OpsDashboardFilter, err error) bool {
	return filter != nil &&
		filter.QueryMode == OpsQueryModeAuto &&
		errors.Is(err, ErrOpsPreaggregatedNotPopulated)
}

func cloneOpsFilterWithMode(filter *OpsDashboardFilter, mode OpsQueryMode) *OpsDashboardFilter {
	if filter == nil {
		return nil
	}
	cloned := *filter
	cloned.QueryMode = mode
	return &cloned
}
