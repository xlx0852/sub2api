// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

// Account quota window sources / close reasons.
const (
	QuotaWindowSourceObserved  = "observed"
	QuotaWindowSourceResetCard = "reset_card"
	QuotaWindowSourceSeed      = "seed"

	QuotaWindowCloseObserved  = "observed_reset"
	QuotaWindowCloseCleared   = "provider_zero_reset"
	QuotaWindowCloseResetCard = "reset_card"
	QuotaWindowCloseReplaced  = "replaced"
)

// AccountQuotaWindow is one real quota period for an account (ledger row).
type AccountQuotaWindow struct {
	ID               int64
	AccountID        int64
	Platform         string
	Kind             string // 5h | 7d | 30d | 24h | ...
	StartAt          time.Time
	EndAt            time.Time
	WindowMinutes    *int
	Source           string
	ClosedReason     string
	UsedPercentOpen  *float64
	UsedPercentClose *float64
	IsOpen           bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountQuotaUsageObservation is one upstream utilization snapshot aligned
// with local consumption ending at ObservedAt.
type AccountQuotaUsageObservation struct {
	ID            int64
	QuotaWindowID int64
	AccountID     int64
	Platform      string
	Kind          string
	ObservedAt    time.Time
	UsedPercent   float64
	Requests      int64
	Tokens        int64
	AccountCost   float64
	StandardCost  float64
	UserCost      float64
	CreatedAt     time.Time
}
