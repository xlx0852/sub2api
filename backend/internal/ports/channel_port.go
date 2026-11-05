// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ChannelRepository interface {
	Create(ctx context.Context, channel *domain.Channel) error
	GetByID(ctx context.Context, id int64) (*domain.Channel, error)
	Update(ctx context.Context, channel *domain.Channel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params domain.PaginationParams, status, search string) ([]domain.Channel, *domain.PaginationResult, error)
	ListAll(ctx context.Context) ([]domain.Channel, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error)

	// 分组关联
	GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error)
	SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error
	GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	// GetChannelIDsByGroupIDs batch resolves group → policy/channel id (0 = none).
	GetChannelIDsByGroupIDs(ctx context.Context, groupIDs []int64) (map[int64]int64, error)
	// ListGroupIDsWithSellPricePolicy returns groupID → policyID for all groups with explicit binding.
	ListGroupIDsWithSellPricePolicy(ctx context.Context) (map[int64]int64, error)
	GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error)

	// 分组平台查询
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)

	// 模型定价
	ListModelPricing(ctx context.Context, channelID int64) ([]domain.ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *domain.ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *domain.ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []domain.ChannelModelPricing) error

	// 统一定价管线（L1 成本基 / L3 修饰因子）
	ListPriceModifiers(ctx context.Context) ([]domain.PriceModifier, error)
	GetPriceCost(ctx context.Context, id int64) (*domain.PriceCost, error)
	ListPriceCosts(ctx context.Context) ([]domain.PriceCost, error)
	UpsertPriceCost(ctx context.Context, cost *domain.PriceCost) (*domain.PriceCost, error)
	DeletePriceCost(ctx context.Context, id int64) error
}
