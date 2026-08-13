// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type AnnouncementRepository interface {
	Create(ctx context.Context, a *domain.Announcement) error
	GetByID(ctx context.Context, id int64) (*domain.Announcement, error)
	Update(ctx context.Context, a *domain.Announcement) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params domain.PaginationParams, filters domain.AnnouncementListFilters) ([]domain.Announcement, *domain.PaginationResult, error)
	ListActive(ctx context.Context, now time.Time) ([]domain.Announcement, error)
}

type AnnouncementReadRepository interface {
	MarkRead(ctx context.Context, announcementID, userID int64, readAt time.Time) error
	GetReadMapByUser(ctx context.Context, userID int64, announcementIDs []int64) (map[int64]time.Time, error)
	GetReadMapByUsers(ctx context.Context, announcementID int64, userIDs []int64) (map[int64]time.Time, error)
	CountByAnnouncementID(ctx context.Context, announcementID int64) (int64, error)
}
