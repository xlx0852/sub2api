// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type PromoCodeRepository interface {
	// 基础 CRUD
	Create(ctx context.Context, code *domain.PromoCode) error
	GetByID(ctx context.Context, id int64) (*domain.PromoCode, error)
	GetByCode(ctx context.Context, code string) (*domain.PromoCode, error)
	GetByCodeForUpdate(ctx context.Context, code string) (*domain.PromoCode, error) // 带行锁的查询，用于并发控制
	Update(ctx context.Context, code *domain.PromoCode) error
	Delete(ctx context.Context, id int64) error

	// 列表查询
	List(ctx context.Context, params domain.PaginationParams) ([]domain.PromoCode, *domain.PaginationResult, error)
	ListWithFilters(ctx context.Context, params domain.PaginationParams, status, search string) ([]domain.PromoCode, *domain.PaginationResult, error)

	// 使用记录
	CreateUsage(ctx context.Context, usage *domain.PromoCodeUsage) error
	GetUsageByPromoCodeAndUser(ctx context.Context, promoCodeID, userID int64) (*domain.PromoCodeUsage, error)
	ListUsagesByPromoCode(ctx context.Context, promoCodeID int64, params domain.PaginationParams) ([]domain.PromoCodeUsage, *domain.PaginationResult, error)

	// 计数操作
	IncrementUsedCount(ctx context.Context, id int64) error
}
