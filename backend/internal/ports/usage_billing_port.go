// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *domain.UsageBillingCommand) (*domain.UsageBillingApplyResult, error)
	ReserveBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
	CaptureBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
	ReleaseBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
}
