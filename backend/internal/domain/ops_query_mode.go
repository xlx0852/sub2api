// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"errors"
)

type OpsQueryMode string

const (
	OpsQueryModeAuto   OpsQueryMode = "auto"
	OpsQueryModeRaw    OpsQueryMode = "raw"
	OpsQueryModePreagg OpsQueryMode = "preagg"
)

// ErrOpsPreaggregatedNotPopulated indicates that raw logs exist for a window, but the
// pre-aggregation tables are not populated yet. This is primarily used to implement
// the forced `preagg` mode UX.
var ErrOpsPreaggregatedNotPopulated = errors.New("ops pre-aggregated tables not populated")

func (m OpsQueryMode) IsValid() bool {
	switch m {
	case OpsQueryModeAuto, OpsQueryModeRaw, OpsQueryModePreagg:
		return true
	default:
		return false
	}
}
