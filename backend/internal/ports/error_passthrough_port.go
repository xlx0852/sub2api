// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ErrorPassthroughRepository interface {
	// List 获取所有规则
	List(ctx context.Context) ([]*domain.ErrorPassthroughRule, error)
	// GetByID 根据 ID 获取规则
	GetByID(ctx context.Context, id int64) (*domain.ErrorPassthroughRule, error)
	// Create 创建规则
	Create(ctx context.Context, rule *domain.ErrorPassthroughRule) (*domain.ErrorPassthroughRule, error)
	// Update 更新规则
	Update(ctx context.Context, rule *domain.ErrorPassthroughRule) (*domain.ErrorPassthroughRule, error)
	// Delete 删除规则
	Delete(ctx context.Context, id int64) error
}

type ErrorPassthroughCache interface {
	// Get 从缓存获取规则列表
	Get(ctx context.Context) ([]*domain.ErrorPassthroughRule, bool)
	// Set 设置缓存
	Set(ctx context.Context, rules []*domain.ErrorPassthroughRule) error
	// Invalidate 使缓存失效
	Invalidate(ctx context.Context) error
	// NotifyUpdate 通知其他实例刷新缓存
	NotifyUpdate(ctx context.Context) error
	// SubscribeUpdates 订阅缓存更新通知
	SubscribeUpdates(ctx context.Context, handler func())
}
