// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type RedeemCodeRepository interface {
	Create(ctx context.Context, code *domain.RedeemCode) error
	CreateBatch(ctx context.Context, codes []domain.RedeemCode) error
	GetByID(ctx context.Context, id int64) (*domain.RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*domain.RedeemCode, error)
	Update(ctx context.Context, code *domain.RedeemCode) error
	BatchUpdate(ctx context.Context, ids []int64, fields domain.RedeemCodeBatchUpdateFields) (int64, error)
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error

	List(ctx context.Context, params domain.PaginationParams) ([]domain.RedeemCode, *domain.PaginationResult, error)
	ListWithFilters(ctx context.Context, params domain.PaginationParams, codeType, status, search string) ([]domain.RedeemCode, *domain.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]domain.RedeemCode, error)
	// ListByUserPaginated returns paginated balance/concurrency history for a specific user.
	// codeType filter is optional - pass empty string to return all types.
	ListByUserPaginated(ctx context.Context, userID int64, params domain.PaginationParams, codeType string) ([]domain.RedeemCode, *domain.PaginationResult, error)
	// SumPositiveBalanceByUser returns the total recharged amount (sum of positive balance values) for a user.
	SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error)
}
