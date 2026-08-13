// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ProxyLatencyCache interface {
	GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*domain.ProxyLatencyInfo, error)
	SetProxyLatency(ctx context.Context, proxyID int64, info *domain.ProxyLatencyInfo) error
}
