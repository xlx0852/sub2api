// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type BatchImageJobLock interface {
	Release(ctx context.Context) error
}

type BatchImageQueue interface {
	Enqueue(ctx context.Context, batchID string) error
	Reserve(ctx context.Context, blockTimeout time.Duration) (domain.ReservedBatchImageJob, error)
	RequeueAfter(ctx context.Context, batchID string, delay time.Duration) error
	Ack(ctx context.Context, batchID string) error
	Heartbeat(ctx context.Context, batchID string) error
	MoveDueDelayedToReady(ctx context.Context, limit int) (int, error)
	RecoverStaleActive(ctx context.Context, staleAfter time.Duration, limit int) (int, error)
	TryAcquireJobLock(ctx context.Context, batchID string, ttl time.Duration) (BatchImageJobLock, bool, error)
}
