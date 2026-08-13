// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UserSubscriptionRepository interface {
	Create(ctx context.Context, sub *domain.UserSubscription) error
	GetByID(ctx context.Context, id int64) (*domain.UserSubscription, error)
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*domain.UserSubscription, error)
	GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*domain.UserSubscription, error)
	GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*domain.UserSubscription, error)
	Update(ctx context.Context, sub *domain.UserSubscription) error
	Delete(ctx context.Context, id int64) error
	Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*domain.UserSubscription, error)

	ListByUserID(ctx context.Context, userID int64) ([]domain.UserSubscription, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]domain.UserSubscription, error)
	ListByGroupID(ctx context.Context, groupID int64, params domain.PaginationParams) ([]domain.UserSubscription, *domain.PaginationResult, error)
	List(ctx context.Context, params domain.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]domain.UserSubscription, *domain.PaginationResult, error)

	ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error
	UpdateStatus(ctx context.Context, subscriptionID int64, status string) error
	UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error

	ActivateWindows(ctx context.Context, id int64, start time.Time) error
	ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error

	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
