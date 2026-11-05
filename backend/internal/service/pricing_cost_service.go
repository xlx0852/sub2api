package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 成本基（L1）的后台管理：列表 / 幂等写入 / 删除。
// 成本基不直接参与扣费，只用于 fx_auto 派生与毛利显示，删错也不会影响账单。

// ListPriceCosts 返回全部成本基（管理后台用，不走热路径缓存）。
func (s *ChannelService) ListPriceCosts(ctx context.Context) ([]domain.PriceCost, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.ListPriceCosts(ctx)
}

// UpsertPriceCost 幂等写入成本基，并让进程内缓存失效。
func (s *ChannelService) UpsertPriceCost(ctx context.Context, cost *domain.PriceCost) (*domain.PriceCost, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	if cost == nil {
		return nil, fmt.Errorf("%w: empty payload", ErrInvalidInput)
	}
	if strings.TrimSpace(cost.Vendor) == "" || strings.TrimSpace(cost.ModelPattern) == "" {
		return nil, fmt.Errorf("%w: vendor and model_pattern are required", ErrInvalidInput)
	}
	if cost.InputPerMTok < 0 || cost.OutputPerMTok < 0 || cost.CachedPerMTok < 0 || cost.CacheWritePerMTok < 0 {
		return nil, fmt.Errorf("%w: prices must not be negative", ErrInvalidInput)
	}
	if cost.DiscountPct < 0 || cost.DiscountPct > 100 {
		return nil, fmt.Errorf("%w: discount_pct must be within 0..100", ErrInvalidInput)
	}
	saved, err := s.repo.UpsertPriceCost(ctx, cost)
	if err != nil {
		return nil, err
	}
	s.invalidatePriceCostCache(saved.ID)
	return saved, nil
}

// DeletePriceCost 删除成本基。
func (s *ChannelService) DeletePriceCost(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("%w: repository unavailable", ErrInvalidInput)
	}
	if err := s.repo.DeletePriceCost(ctx, id); err != nil {
		return err
	}
	s.invalidatePriceCostCache(id)
	return nil
}

func (s *ChannelService) invalidatePriceCostCache(id int64) {
	s.costsMu.Lock()
	delete(s.costs, id)
	s.costsMu.Unlock()
}
