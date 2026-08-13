// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
)

// Internal500CounterCache 追踪 Antigravity 账号连续 INTERNAL 500 失败轮数
type Internal500CounterCache interface {
	// IncrementInternal500Count 原子递增计数并返回当前值
	IncrementInternal500Count(ctx context.Context, accountID int64) (int64, error)
	// ResetInternal500Count 清零计数器（成功响应时调用）
	ResetInternal500Count(ctx context.Context, accountID int64) error
}
