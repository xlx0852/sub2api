package scheduling

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AccountLookup 是调度快照/仓库的只读取号端口。
type AccountLookup interface {
	GetAccount(ctx context.Context, accountID int64) (*domain.Account, error)
}

// ConfigOrDefault 在配置缺失时回落到 DefaultConfig。
func ConfigOrDefault(cfg *config.GatewaySchedulingConfig) config.GatewaySchedulingConfig {
	if cfg == nil {
		return DefaultConfig()
	}
	return *cfg
}

// LookupSchedulable 优先读调度快照，否则回落仓库。
// snapshot 为 nil 时与原先“直接走 repo.GetByID”一致。
func LookupSchedulable(ctx context.Context, snapshot AccountLookup, fallback AccountLookup, accountID int64) (*domain.Account, error) {
	if snapshot != nil {
		return snapshot.GetAccount(ctx, accountID)
	}
	return fallback.GetAccount(ctx, accountID)
}

// HydrateSelected 在存在调度快照时用完整账号覆盖精简快照（补回凭证）。
// snapshot 为 nil 时原样返回 account。kind 仅用于 hydrate 失败错误文案
// （gateway / openai / gemini），保持各调度器原有错误字符串。
func HydrateSelected(ctx context.Context, snapshot AccountLookup, account *domain.Account, kind string) (*domain.Account, error) {
	if account == nil || snapshot == nil {
		return account, nil
	}
	hydrated, err := snapshot.GetAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if hydrated == nil {
		return nil, fmt.Errorf("selected %s account %d not found during hydration", kind, account.ID)
	}
	return hydrated, nil
}

// NewResult 构造普通选号结果。
func NewResult(account *domain.Account, acquired bool, release func(), waitPlan *WaitPlan) *Result {
	return &Result{
		Account:  account,
		Acquired: acquired,
		Release:  release,
		WaitPlan: waitPlan,
	}
}

// NewModelLimitedResult 构造模型级预算已满的拒绝结果。
// WaitPlan 恒为 nil，调用方应直接让路，不进入账号等待队列。
func NewModelLimitedResult(account *domain.Account) *Result {
	return &Result{
		Account:      account,
		Acquired:     false,
		ModelLimited: true,
	}
}

// ReleaseOnError 在 hydrate/构造失败时立即释放已抢到的并发槽，避免等到 Redis TTL。
func ReleaseOnError(release func(), err error) {
	if err != nil && release != nil {
		release()
	}
}
