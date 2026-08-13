// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"
)

// BatchImageJobLockRefresher 是可选的锁续期能力；由具体锁实现按需提供。
type BatchImageJobLockRefresher interface {
	Refresh(ctx context.Context, ttl time.Duration) error
}
