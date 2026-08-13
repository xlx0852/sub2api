// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
)

// RedeemUserAdjustmentRepository provides the atomic, floor-at-zero updates
// used by negative-value redeem codes. It is intentionally narrower than
// UserRepository because normal usage billing is allowed to overdraw.
type RedeemUserAdjustmentRepository interface {
	ApplyRedeemBalanceAdjustment(ctx context.Context, id int64, delta float64) error
	ApplyRedeemConcurrencyAdjustment(ctx context.Context, id int64, delta int) error
}
