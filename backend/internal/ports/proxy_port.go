// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ProxyRepository interface {
	Create(ctx context.Context, proxy *domain.Proxy) error
	GetByID(ctx context.Context, id int64) (*domain.Proxy, error)
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Proxy, error)
	Update(ctx context.Context, proxy *domain.Proxy) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params domain.PaginationParams) ([]domain.Proxy, *domain.PaginationResult, error)
	ListWithFilters(ctx context.Context, params domain.PaginationParams, protocol, status, search string) ([]domain.Proxy, *domain.PaginationResult, error)
	ListWithFiltersAndAccountCount(ctx context.Context, params domain.PaginationParams, protocol, status, search string) ([]domain.ProxyWithAccountCount, *domain.PaginationResult, error)
	ListActive(ctx context.Context) ([]domain.Proxy, error)
	ListActiveWithAccountCount(ctx context.Context) ([]domain.ProxyWithAccountCount, error)

	ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error)
	CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error)
	ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]domain.ProxyAccountSummary, error)

	SweepExpiredProxies(ctx context.Context, now time.Time) (changed int64, err error)
	ListAllForFallback(ctx context.Context) ([]domain.Proxy, error)
	CountExpired(ctx context.Context) (int64, error)
	CountExpiringSoon(ctx context.Context, now time.Time) (int64, error)
}
