// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
)

// UserMsgQueueCache 用户消息串行队列 Redis 缓存接口
type UserMsgQueueCache interface {
	// AcquireLock 尝试获取账号级串行锁
	AcquireLock(ctx context.Context, accountID int64, requestID string, lockTtlMs int) (acquired bool, err error)
	// ReleaseLock 释放锁并记录完成时间
	ReleaseLock(ctx context.Context, accountID int64, requestID string) (released bool, err error)
	// GetLastCompletedMs 获取上次完成时间（毫秒时间戳，Redis TIME 源）
	GetLastCompletedMs(ctx context.Context, accountID int64) (int64, error)
	// GetCurrentTimeMs 获取 Redis 服务器当前时间（毫秒），与 ReleaseLock 记录的时间源一致
	GetCurrentTimeMs(ctx context.Context) (int64, error)
	// ReconcileExpiredLockCandidates 处理锁索引中的到期候选，按真实 PTTL 清理或刷新索引
	ReconcileExpiredLockCandidates(ctx context.Context, maxCount int) (cleaned int, err error)
}
