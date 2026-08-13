// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AccountQuotaWindowRepository persists real quota-window history.
type AccountQuotaWindowRepository interface {
	ListByAccount(ctx context.Context, accountID int64, kind string, limit int) ([]*domain.AccountQuotaWindow, error)
	ListOpenByAccount(ctx context.Context, accountID int64) ([]*domain.AccountQuotaWindow, error)
	GetOpen(ctx context.Context, accountID int64, kind string) (*domain.AccountQuotaWindow, error)
	// CloseAndOpen atomically closes the open row (if any) and inserts a new open row.
	CloseAndOpen(ctx context.Context, closeID int64, closeEnd time.Time, closeReason string, closeUsed *float64, open *domain.AccountQuotaWindow) (*domain.AccountQuotaWindow, error)
	// UpsertOpenRefresh updates end/used on the current open window without closing.
	UpsertOpenRefresh(ctx context.Context, accountID int64, kind string, endAt time.Time, used *float64, windowMinutes *int) error
	InsertOpen(ctx context.Context, w *domain.AccountQuotaWindow) (*domain.AccountQuotaWindow, error)
	CloseOpen(ctx context.Context, accountID int64, kind string, endAt time.Time, reason string, used *float64) error
}
