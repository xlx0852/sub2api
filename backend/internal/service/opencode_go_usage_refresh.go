package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// OpenCode Go 用量窗口的自动刷新。
//
// 上游 v0.2.8 的策略是**活动驱动**（模型请求带 debounce 触发）+ 5 分钟硬下限、
// 15 分钟默认上限。本仓没有"请求驱动"的钩子，所以这里做**等价数字的简化版**：
//
//   - 扫描周期 5 分钟（= 上游硬下限，保证不会比上游更频繁地打官方接口）；
//   - 陈旧阈值 15 分钟（= 上游默认上限），超过才查；
//   - 失败退避：查失败后同样按 15 分钟退避，避免对 401/403 反复试探。
//
// 与"请求驱动"的差别只在**时效性**（最多晚 15 分钟），不会造成额外请求量。
const (
	openCodeGoUsageScanInterval   = 5 * time.Minute
	openCodeGoUsageStaleAfter     = 15 * time.Minute
	openCodeGoUsageRefreshSpacing = 400 * time.Millisecond
)

// openCodeGoUsageNeedsRefresh 判断账号的用量快照是否需要重查。
//
// 快照缺失、或上次抓取（无论成功失败）已超过 staleAfter → 需要。
// 失败也参与计时，因此失败不会变成"每次都重试"。
func openCodeGoUsageNeedsRefresh(account *Account, now time.Time, staleAfter time.Duration) bool {
	if staleAfter <= 0 {
		staleAfter = openCodeGoUsageStaleAfter
	}
	snapshot := openCodeGoUsageFromExtra(account)
	if snapshot == nil || snapshot.FetchedAt.IsZero() {
		return true
	}
	// 抓取时间在未来（时钟回拨/脏数据）时也重查，别把账号永久钉死。
	if snapshot.FetchedAt.After(now) {
		return true
	}
	return now.Sub(snapshot.FetchedAt) >= staleAfter
}

func (s *OpenAIGatewayService) startOpenCodeGoUsageRefresher() {
	if s == nil || s.accountRepo == nil {
		return
	}
	s.openCodeGoUsageRefreshOnce.Do(func() {
		go s.runOpenCodeGoUsageRefresher()
	})
}

func (s *OpenAIGatewayService) runOpenCodeGoUsageRefresher() {
	ticker := time.NewTicker(openCodeGoUsageScanInterval)
	defer ticker.Stop()
	// 启动后先扫一轮：进程重启不该让用量停更一个周期。
	s.safeRefreshOpenCodeGoUsage()
	for range ticker.C {
		s.safeRefreshOpenCodeGoUsage()
	}
}

// safeRefreshOpenCodeGoUsage 包住一次扫描，兜住 panic。
//
// 这是后台旁路任务（只是顺带刷新用量窗口），它身上的未捕获 panic 会**直接终止整个进程**，
// 代价远远大于"本轮用量没刷到"。已实测：只要 accountRepo 是实现了部分方法的 stub
// （内嵌 nil 接口），ListByPlatform 就会 nil 解引用 panic，把调用方整个测试包打崩。
func (s *OpenAIGatewayService) safeRefreshOpenCodeGoUsage() {
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf("service.opencode_go", "OpenCode Go usage refresh panic recovered: %v", r)
		}
	}()
	s.refreshOpenCodeGoUsageAccounts(context.Background())
}

// refreshOpenCodeGoUsageAccounts 扫描一次并刷新到期的账号。
//
// 只看 OpenAI 平台的 API-key 账号，且必须是"base URL 精确指向 OpenCode Go 官方基址"
// 的那一类——判定复用 IsOpenCodeGoAccount，绝不对其它账号发请求（避免把别家 key 发出去）。
func (s *OpenAIGatewayService) refreshOpenCodeGoUsageAccounts(ctx context.Context) {
	if s == nil || s.accountRepo == nil || ctx.Err() != nil {
		return
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	refreshed, failed := 0, 0
	for i := range accounts {
		account := &accounts[i]
		if !IsOpenCodeGoAccount(account) || !openCodeGoUsageNeedsRefresh(account, now, openCodeGoUsageStaleAfter) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		usage := probeOpenCodeGoUsage(ctx, s.httpUpstream, account)
		s.persistOpenCodeGoUsage(account, usage)
		if usage != nil && usage.Error == "" {
			refreshed++
		} else {
			failed++
		}
		// 账号之间留间隔：官方接口是外部依赖，不该被我们连打。
		time.Sleep(openCodeGoUsageRefreshSpacing)
	}
	if refreshed > 0 || failed > 0 {
		logger.LegacyPrintf("service.opencode_go_usage",
			"[OpenCodeGoUsage] auto refresh refreshed=%d failed=%d", refreshed, failed)
	}
}
