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
	// ListGroupRefsByPolicy returns policyID → bound groups (id/name/rate) for the pricing overview.
	ListGroupRefsByPolicy(ctx context.Context) (map[int64][]domain.GroupPolicyBinding, error)
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
	// GetModelPrice 返回模型在分组下的价格行（精确分组行优先，其次平台默认行）。
	GetModelPrice(ctx context.Context, modelID string, groupID *int64) (*domain.ModelPriceRow, error)

	// —— 模型注册表后台管理 ——
	ListModelRegistry(ctx context.Context, platform, search string) ([]domain.ModelRegistryEntry, error)
	UpsertModelRegistry(ctx context.Context, m *domain.ModelRegistryEntry, expectedVersion int64) (int64, error)
	ListModelAliases(ctx context.Context, modelID string) ([]domain.ModelAliasEntry, error)
	AliasConflict(ctx context.Context, alias, platform, modelID string) (string, error)
	ReplaceModelAliases(ctx context.Context, modelID string, aliases []domain.ModelAliasEntry) error
	UpsertModelPrice(ctx context.Context, row *domain.ModelPriceRow) error
	DeleteModelPrice(ctx context.Context, modelID string, groupID *int64) error
	ListModelPrices(ctx context.Context, modelID string) ([]domain.ModelPriceRow, error)
	ListPriceCosts(ctx context.Context) ([]domain.PriceCost, error)
	UpsertPriceCost(ctx context.Context, cost *domain.PriceCost) (*domain.PriceCost, error)
	DeletePriceCost(ctx context.Context, id int64) error
}
