// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
)

type BatchImageDownloadLimiter interface {
	Acquire(ctx context.Context, userID string, kind string) (BatchImageDownloadPermit, error)
}

type BatchImageDownloadPermit interface {
	Release(ctx context.Context) error
}
