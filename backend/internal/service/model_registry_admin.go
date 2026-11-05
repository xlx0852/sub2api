package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 模型注册表后台管理：护栏集中在这里。
//
// 三条硬规则（openspec: move-model-registry-to-db）：
//   1. 未登记的模型不能配价（否则"模型存在"又有两个来源）；
//   2. 价格行不能"存在但没有任何价"（历史上正是它导致 0 元计费）；
//   3. 别名不能同时指向两个模型。

// ListModelRegistry 返回模型清单。
func (s *ChannelService) ListModelRegistry(ctx context.Context, platform, search string) ([]domain.ModelRegistryEntry, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.ListModelRegistry(ctx, platform, search)
}

// UpsertModelRegistry 新增/更新一个模型（乐观锁：expectedVersion>0 时必须匹配）。
func (s *ChannelService) UpsertModelRegistry(ctx context.Context, m *domain.ModelRegistryEntry, expectedVersion int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	if m == nil || strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("%w: model id is required", ErrInvalidInput)
	}
	m.ID = strings.TrimSpace(m.ID)
	m.Platform = strings.TrimSpace(m.Platform)
	if m.Retired && m.Enabled {
		return fmt.Errorf("%w: retired model cannot stay enabled", ErrInvalidInput)
	}
	_, err := s.repo.UpsertModelRegistry(ctx, m, expectedVersion)
	return err
}

// ReplaceModelAliases 覆盖某模型的别名集合；任一别名已被别的模型占用则整体拒绝。
func (s *ChannelService) ReplaceModelAliases(ctx context.Context, modelID string, aliases []domain.ModelAliasEntry) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("%w: model id is required", ErrInvalidInput)
	}
	cleaned := make([]domain.ModelAliasEntry, 0, len(aliases))
	for _, a := range aliases {
		a.Alias = strings.TrimSpace(a.Alias)
		a.Platform = strings.TrimSpace(a.Platform)
		if a.Alias == "" {
			continue
		}
		if a.ModelID == "" {
			a.ModelID = modelID
		}
		if a.ModelID != modelID {
			return fmt.Errorf("%w: alias entry must point at %s", ErrInvalidInput, modelID)
		}
		if a.Kind == "" {
			a.Kind = "alias"
		}
		switch a.Kind {
		case "alias", "id_override", "id_reverse", "retired":
		default:
			return fmt.Errorf("%w: unsupported alias kind %q", ErrInvalidInput, a.Kind)
		}
		if existing, err := s.repo.AliasConflict(ctx, a.Alias, a.Platform, modelID); err != nil {
			return err
		} else if existing != "" {
			return fmt.Errorf("%w: alias %q already points at %s", ErrInvalidInput, a.Alias, existing)
		}
		cleaned = append(cleaned, a)
	}
	return s.repo.ReplaceModelAliases(ctx, modelID, cleaned)
}

// UpsertModelPrice 写入一条价格行（groupID 为空 = 平台默认价）。
func (s *ChannelService) UpsertModelPrice(ctx context.Context, row *domain.ModelPriceRow) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	if row == nil || strings.TrimSpace(row.ModelID) == "" {
		return fmt.Errorf("%w: model id is required", ErrInvalidInput)
	}
	row.ModelID = strings.TrimSpace(row.ModelID)

	// 规则 1：模型必须先登记。
	models, err := s.repo.ListModelRegistry(ctx, "", row.ModelID)
	if err != nil {
		return err
	}
	registered := false
	for _, m := range models {
		if m.ID == row.ModelID {
			registered = true
			break
		}
	}
	if !registered {
		return fmt.Errorf("%w: %s", ErrModelNotRegistered, row.ModelID)
	}

	// 规则 2：不能存在"没有价"的行。
	if row.IsEmpty() {
		return fmt.Errorf("%w: a price row must have at least one price", ErrInvalidInput)
	}

	// fx_auto 必须自带成本基与汇率，否则运行时无法派生（fail-closed）。
	if NormalizePricingFxMode(row.FxMode) == FxModeAuto {
		if row.CostID == nil || row.FxRate == nil || *row.FxRate <= 0 {
			return fmt.Errorf("%w: fx_auto requires cost_id and a positive fx_rate", ErrInvalidInput)
		}
		if s.GetPriceCost(ctx, *row.CostID) == nil {
			return fmt.Errorf("%w: cost basis %d not found", ErrPriceCostMissing, *row.CostID)
		}
	}
	if row.BillingMode == "" {
		row.BillingMode = "token"
	}
	if row.Currency == "" {
		row.Currency = string(domain.PricingCurrencyCNY)
	}
	row.Enabled = true
	return s.repo.UpsertModelPrice(ctx, row)
}

// DeleteModelPrice 删除一条价格行。
func (s *ChannelService) DeleteModelPrice(ctx context.Context, modelID string, groupID *int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	return s.repo.DeleteModelPrice(ctx, strings.TrimSpace(modelID), groupID)
}

// ErrModelNotRegistered 表示尝试为未登记的模型配价。
var ErrModelNotRegistered = fmt.Errorf("model is not registered")

// ListModelAliasesFor 返回某模型的别名（后台展示用）。
func (s *ChannelService) ListModelAliasesFor(ctx context.Context, modelID string) ([]domain.ModelAliasEntry, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.ListModelAliases(ctx, strings.TrimSpace(modelID))
}

// ListModelPricesFor 返回某模型的全部价格行（默认价排在最前）。
func (s *ChannelService) ListModelPricesFor(ctx context.Context, modelID string) ([]domain.ModelPriceRow, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.ListModelPrices(ctx, strings.TrimSpace(modelID))
}
