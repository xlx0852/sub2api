package scheduling

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// SlotResult 是模型感知并发槽位的获取结果。
// 字段与现有 service.AcquireResult 对齐，便于两套调度器无损适配。
type SlotResult struct {
	Acquired     bool
	Release      func()
	ModelLimited bool
}

// ModelAwareSlotAcquirer 是并发服务的调度侧端口。
// 现有 ConcurrencyService 经 service 适配器实现，不把 service 类型引入 app。
type ModelAwareSlotAcquirer interface {
	AcquireModelAwareAccountSlot(ctx context.Context, requestedModel string, accountID int64, maxConcurrency int) (*SlotResult, error)
}

// PassthroughSlot 是未接入并发服务时的 fail-open 结果：视为已获取，释放为空操作。
func PassthroughSlot() SlotResult {
	return SlotResult{Acquired: true, Release: func() {}}
}

// TryAcquireAccountSlot 统一两套调度器的槽位获取入口。
// acquirer 为 nil 时 fail-open，不改变“无并发服务即放行”的语义。
func TryAcquireAccountSlot(ctx context.Context, acquirer ModelAwareSlotAcquirer, requestedModel string, accountID int64, maxConcurrency int) (*SlotResult, error) {
	if acquirer == nil {
		slot := PassthroughSlot()
		return &slot, nil
	}
	// 请求耗时详情：账号并发准入与等待（Anthropic/Gemini 等网关路径）。
	started := time.Now()
	result, err := acquirer.AcquireModelAwareAccountSlot(ctx, requestedModel, accountID, maxConcurrency)
	if collector := requesttiming.FromContext(ctx); collector != nil {
		collector.SetAccountQueueMs(time.Since(started))
	}
	return result, err
}

// IsExcluded 判断账号是否在本轮调度排除集中。
// excludedIDs 为 nil 时视为未排除（与 Go 对 nil map 的读语义一致）。
func IsExcluded(excludedIDs map[int64]struct{}, accountID int64) bool {
	if excludedIDs == nil {
		return false
	}
	_, excluded := excludedIDs[accountID]
	return excluded
}

// CloneExcluded 复制排除集；空输入保持 nil，避免无谓分配。
func CloneExcluded(excludedIDs map[int64]struct{}) map[int64]struct{} {
	if len(excludedIDs) == 0 {
		return nil
	}
	cloned := make(map[int64]struct{}, len(excludedIDs))
	for accountID := range excludedIDs {
		cloned[accountID] = struct{}{}
	}
	return cloned
}

// AddExcluded 以写时初始化方式加入排除账号并返回集合。
func AddExcluded(excludedIDs map[int64]struct{}, accountID int64) map[int64]struct{} {
	if excludedIDs == nil {
		excludedIDs = make(map[int64]struct{})
	}
	excludedIDs[accountID] = struct{}{}
	return excludedIDs
}
