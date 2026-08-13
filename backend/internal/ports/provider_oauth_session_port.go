// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ProviderOAuthSessionStore interface {
	Create(ctx context.Context, session *domain.ProviderOAuthSession, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (*domain.ProviderOAuthSession, error)
	AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*domain.ProviderOAuthSessionLease, error)
	CommitPoll(ctx context.Context, lease *domain.ProviderOAuthSessionLease, updated *domain.ProviderOAuthSession) (bool, error)
	Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error)
	ConsumeAuthorized(ctx context.Context, sessionID string) (*domain.ProviderOAuthSession, error)
	Delete(ctx context.Context, sessionID string) error
}
