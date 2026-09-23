package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// openCodeGoUsagePersistMu 串行化同一进程内的用量落库，避免并发写覆盖。
var openCodeGoUsagePersistMu sync.Mutex

// QueryOpenCodeGoUsage 由管理端触发：查一次 OpenCode Go 官方用量窗口并落库。
//
// 放在 AccountTestService 而不是网关服务：管理端 handler 只持有前者，
// 而两者都具备 HTTPUpstream + accountRepo，不需要为一个查询端点去改依赖注入图。
func (s *AccountTestService) QueryOpenCodeGoUsage(ctx context.Context, accountID int64) (*OpenCodeGoUsage, error) {
	if s == nil || accountID <= 0 {
		return nil, errors.New("invalid account id")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !IsOpenCodeGoAccount(account) {
		// 明确区分"不是这类账号"与"查询失败"：前端要据此决定是否展示该面板。
		return nil, ErrOpenCodeGoUsageNotApplicable
	}
	usage := probeOpenCodeGoUsage(ctx, s.httpUpstream, account)
	if usage == nil {
		return nil, errors.New("opencode go usage probe returned nothing")
	}
	// 失败也要落库：面板需要区分"没查过"与"查了但失败"。
	stored := s.persistAccountOpenCodeGoUsage(account, usage)
	if usage.Error != "" {
		return stored, fmt.Errorf("opencode go usage: %s", usage.Error)
	}
	return stored, nil
}

// persistAccountOpenCodeGoUsage 把快照写入账号 extra 并落库。
// **只写窗口数据与短原因，绝不写 API key。**
func (s *AccountTestService) persistAccountOpenCodeGoUsage(account *Account, usage *OpenCodeGoUsage) *OpenCodeGoUsage {
	return persistOpenCodeGoUsageSnapshot(account, usage, func(id int64, updates map[string]any) {
		openCodeGoUsagePersistMu.Lock()
		defer openCodeGoUsagePersistMu.Unlock()
		_ = s.accountRepo.UpdateExtra(context.Background(), id, updates)
	})
}
