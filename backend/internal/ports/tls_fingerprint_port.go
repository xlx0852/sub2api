// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type TLSFingerprintProfileRepository interface {
	List(ctx context.Context) ([]*domain.TLSFingerprintProfile, error)
	GetByID(ctx context.Context, id int64) (*domain.TLSFingerprintProfile, error)
	Create(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error)
	Update(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error)
	Delete(ctx context.Context, id int64) error
}

type TLSFingerprintProfileCache interface {
	Get(ctx context.Context) ([]*domain.TLSFingerprintProfile, bool)
	Set(ctx context.Context, profiles []*domain.TLSFingerprintProfile) error
	Invalidate(ctx context.Context) error
	NotifyUpdate(ctx context.Context) error
	SubscribeUpdates(ctx context.Context, handler func())
}
