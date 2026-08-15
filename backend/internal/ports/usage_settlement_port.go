package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UsageSettlementClaimRequest struct {
	Owner         string
	Limit         int
	LeaseDuration time.Duration
	GroupIDs      []int64
}

type UsageSettlementBacklogStats struct {
	Started                int64
	Ready                  int64
	Processing             int64
	Retryable              int64
	ReconciliationRequired int64
	OldestPendingAt        *time.Time
}

type UsageSettlementRepository interface {
	CreateStarted(ctx context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error)
	ObserveProviderRequestID(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, providerRequestID string) (*domain.UsageSettlementAttempt, error)
	MarkReady(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint string, payload *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error)
	Claim(ctx context.Context, request UsageSettlementClaimRequest) ([]domain.UsageSettlementAttempt, error)
	ExpireStarted(ctx context.Context, now time.Time, limit int) (int64, error)
	ListExpiredRecoverable(ctx context.Context, now time.Time, limit int) ([]domain.UsageSettlementAttempt, error)
	BacklogStats(ctx context.Context) (*UsageSettlementBacklogStats, error)
	MarkRetryable(ctx context.Context, id int64, owner string, nextAttemptAt time.Time, code, message string) error
	MarkReconciliationRequired(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, code, message string) error
	MarkFailedTerminal(ctx context.Context, id int64, owner, code, message string) error
	MarkSettled(ctx context.Context, id int64, owner string) error
	GetByIdentity(ctx context.Context, billingIdentity string, apiKeyID int64) (*domain.UsageSettlementAttempt, error)
}

// UsageSettlementCoordinator is the application boundary used by gateway
// services. Repositories remain hidden behind the durable settlement worker.
type UsageSettlementCoordinator interface {
	Enabled() bool
	Authoritative() bool
	EnabledFor(groupID *int64) bool
	AuthoritativeFor(groupID *int64) bool
	StartAttempt(ctx context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error)
	ObserveProviderRequestID(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, providerRequestID string) (*domain.UsageSettlementAttempt, error)
	HandoffReady(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint string, payload *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error)
	Wake()
}

type UsageSettlementAppliedObserver interface {
	AfterUsageSettlement(ctx context.Context, attempt *domain.UsageSettlementAttempt, result *domain.UsageSettlementApplyResult)
}

type UsageSettlementTerminalRecoverer interface {
	RecoverUsageSettlement(ctx context.Context, attempt *domain.UsageSettlementAttempt) (bool, error)
}
