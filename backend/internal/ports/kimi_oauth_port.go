// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type KimiDeviceSessionStore interface {
	Create(ctx context.Context, session *domain.KimiDeviceSession, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (*domain.KimiDeviceSession, error)
	AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*domain.KimiDevicePollLease, error)
	CommitPoll(ctx context.Context, lease *domain.KimiDevicePollLease, updated *domain.KimiDeviceSession) (bool, error)
	ConsumeAuthorized(ctx context.Context, sessionID string) (*domain.KimiDeviceSession, error)
	Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error)
	Delete(ctx context.Context, sessionID string) error
}
