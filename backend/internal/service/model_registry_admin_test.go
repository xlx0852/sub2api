//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func registryStub(models []domain.ModelRegistryEntry) *ChannelService {
	return &ChannelService{
		repo: &mockChannelRepository{
			listModelRegistryFn: func(context.Context, string, string) ([]domain.ModelRegistryEntry, error) {
				return models, nil
			},
		},
	}
}

// 规则 2：一行价格都没有 → 拒绝（历史上这种行会把请求按 0 元结算）。
func TestUpsertModelPriceRejectsEmptyRow(t *testing.T) {
	svc := registryStub([]domain.ModelRegistryEntry{{ID: "m-1", Platform: "openai", Enabled: true}})
	err := svc.UpsertModelPrice(context.Background(), &domain.ModelPriceRow{ModelID: "m-1"})
	require.ErrorIs(t, err, ErrEmptyPriceRow)
}

// 规则 1：未登记模型禁止配价。
func TestUpsertModelPriceRejectsUnregisteredModel(t *testing.T) {
	svc := registryStub(nil)
	price := 1e-6
	err := svc.UpsertModelPrice(context.Background(), &domain.ModelPriceRow{ModelID: "ghost", InputPrice: &price})
	require.ErrorIs(t, err, ErrModelNotRegistered)
}

// fx_auto 缺成本基/汇率 → 拒绝（否则运行时派生失败）。
func TestUpsertModelPriceRequiresCostForFxAuto(t *testing.T) {
	svc := registryStub([]domain.ModelRegistryEntry{{ID: "m-1", Platform: "openai", Enabled: true}})
	price := 1e-6
	err := svc.UpsertModelPrice(context.Background(), &domain.ModelPriceRow{
		ModelID: "m-1", InputPrice: &price, FxMode: domain.PricingFxModeAuto,
	})
	require.ErrorIs(t, err, ErrInvalidFxAutoPrice)
}

// 正常行（手工价）应通过校验。
func TestUpsertModelPriceAcceptsManualRow(t *testing.T) {
	svc := registryStub([]domain.ModelRegistryEntry{{ID: "m-1", Platform: "openai", Enabled: true}})
	price := 1e-6
	err := svc.UpsertModelPrice(context.Background(), &domain.ModelPriceRow{
		ModelID: "m-1", InputPrice: &price, FxMode: domain.PricingFxModeManual,
	})
	require.NoError(t, err)
}

// 规则 3：别名已指向别的模型 → 整体拒绝。
func TestReplaceModelAliasesRejectsConflict(t *testing.T) {
	svc := &ChannelService{repo: &mockChannelRepository{
		aliasConflictFn: func(context.Context, string, string, string) (string, error) { return "other-model", nil },
	}}
	err := svc.ReplaceModelAliases(context.Background(), "m-1", []domain.ModelAliasEntry{{Alias: "a", Platform: "openai"}})
	require.ErrorIs(t, err, ErrInvalidInput)
}

// retired 与 enabled 不能同时为真。
func TestUpsertModelRegistryRejectsRetiredEnabled(t *testing.T) {
	svc := registryStub(nil)
	err := svc.UpsertModelRegistry(context.Background(), &domain.ModelRegistryEntry{
		ID: "m-1", Platform: "openai", Retired: true, Enabled: true,
	}, 0)
	require.ErrorIs(t, err, ErrRetiredModelEnabled)
}
