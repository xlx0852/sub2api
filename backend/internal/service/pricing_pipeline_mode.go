package service

import (
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// groupIDValue 把可空分组 id 折成 int64（nil → 0）。
func groupIDValue(id *int64) int64 {
	if id == nil || *id <= 0 {
		return 0
	}
	return *id
}

// shadowDiffSeen 保证灰度期每个 (model, group) 组合只打一条差异日志。
var shadowDiffSeen sync.Map

// logShadowDiff 记录 legacy 与 unified 两条管线的价格差异（仅观测，不影响扣费）。
func (s *BillingService) logShadowDiff(model string, groupID int64, legacy, unified *ModelPricing, applied []AppliedModifier) {
	if legacy == nil || unified == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(model)) + "|" + strconv.FormatInt(groupID, 10)
	if _, loaded := shadowDiffSeen.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	modifierIDs := make([]int64, 0, len(applied))
	for _, a := range applied {
		modifierIDs = append(modifierIDs, a.ID)
	}
	slog.Warn("pricing_pipeline.shadow_diff",
		"model", model,
		"group_id", groupID,
		"legacy_input_per_token", legacy.InputPricePerToken,
		"unified_input_per_token", unified.InputPricePerToken,
		"legacy_output_per_token", legacy.OutputPricePerToken,
		"unified_output_per_token", unified.OutputPricePerToken,
		"legacy_cache_read_per_token", legacy.CacheReadPricePerToken,
		"unified_cache_read_per_token", unified.CacheReadPricePerToken,
		"modifier_ids", modifierIDs,
	)
}
