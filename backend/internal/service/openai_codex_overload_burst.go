package service

import (
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 容量过载的爆发窗口与重试预算。
//
// 依据（本地 20159 个 turn / 190 次 server_is_overloaded 的实测）：
//   - 过载是上游容量的瞬时抽签：同一账号同一会话在数十秒内会交替成败；
//   - 但过载在时间上强聚集：57 个爆发簇，最大 13 次 / 42 分钟，
//     且每次过载后 10-40 分钟是高危窗口（其余时段整周为 0）。
//
// 因此「一次过载之后」比「任意时刻」更值得多给重试预算：此时重试是低成本高期望的
// （过载发生在首包前时上游未做任何计算），而窗口外则应保持基线预算避免无谓放大负载。
const (
	// openAICodexOverloadBurstWindow 过载爆发窗口：距上次过载在该时长内视为同一簇。
	openAICodexOverloadBurstWindow = 30 * time.Minute
	// openAICodexOverloadBurstRetryBonus 爆发窗口内对同账号重试上限的加成。
	// 保持很小：这只用于把「明显还在爆发中」的请求多给几次重抽机会，
	// 不是放开重试总数。
	openAICodexOverloadBurstRetryBonus = 2
	// openAICodexOverloadBurstMaxEntries 追踪表的容量上限，避免账号数极多时无界增长。
	openAICodexOverloadBurstMaxEntries = 4096
)

type codexOverloadBurstState struct {
	// windowStart 当前爆发窗口的起点（首个过载时刻）。
	windowStart time.Time
	// attempts 窗口内累计过的过载次数。
	attempts int
	// lastSeen 最近一次过载时刻，用于剪枝。
	lastSeen time.Time
}

var openAICodexOverloadBurst struct {
	mu    sync.Mutex
	state map[int64]codexOverloadBurstState
}

// OnOpenAICodexOverload 记录一次明确的容量过载，返回它在当前爆发窗口内的序号。
//
// 序号语义：窗口外（或首个）过载返回 1 并开启新窗口；窗口内每次递增。
// 该序号仅用于结构化观测与预算判断，不参与上游请求内容。
func (s *OpenAIGatewayService) OnOpenAICodexOverload(accountID int64) int {
	if accountID <= 0 {
		return 0
	}
	now := time.Now()
	openAICodexOverloadBurst.mu.Lock()
	defer openAICodexOverloadBurst.mu.Unlock()
	if openAICodexOverloadBurst.state == nil {
		openAICodexOverloadBurst.state = make(map[int64]codexOverloadBurstState, 16)
	}
	st, ok := openAICodexOverloadBurst.state[accountID]
	if !ok || now.Sub(st.lastSeen) > openAICodexOverloadBurstWindow {
		if len(openAICodexOverloadBurst.state) >= openAICodexOverloadBurstMaxEntries {
			pruneOpenAICodexOverloadBurstLocked(now)
		}
		st = codexOverloadBurstState{windowStart: now}
	}
	st.attempts++
	st.lastSeen = now
	openAICodexOverloadBurst.state[accountID] = st
	return st.attempts
}

// OpenAICodexOverloadBurstAttempt 返回该账号当前爆发窗口内已记录的过载次数（窗口外为 0）。
func (s *OpenAIGatewayService) OpenAICodexOverloadBurstAttempt(accountID int64) int {
	if accountID <= 0 {
		return 0
	}
	now := time.Now()
	openAICodexOverloadBurst.mu.Lock()
	defer openAICodexOverloadBurst.mu.Unlock()
	st, ok := openAICodexOverloadBurst.state[accountID]
	if !ok || now.Sub(st.lastSeen) > openAICodexOverloadBurstWindow {
		return 0
	}
	return st.attempts
}

// OpenAICodexOverloadRetryBudget 返回本次同账号重试应使用的上限。
//
// 只有「明确的容量过载 + 处于爆发窗口内」才放宽到 base+bonus；
// 其余情况（包括窗口外过载、以及非过载的可重试错误）一律返回 base。
// base < 0 视为 0；加成后仍受 base+openAICodexOverloadBurstRetryBonus 封顶。
func (s *OpenAIGatewayService) OpenAICodexOverloadRetryBudget(accountID int64, base int, overloaded bool) int {
	if base < 0 {
		base = 0
	}
	if !overloaded || accountID <= 0 || !s.CodexOverloadIdentityRotationEnabled() {
		return base
	}
	if s.OpenAICodexOverloadBurstAttempt(accountID) < 2 {
		// 首次过载不额外放宽：这是抽签而非趋势，基线预算已经覆盖。
		return base
	}
	return base + openAICodexOverloadBurstRetryBonus
}

// CodexOverloadIdentityRotationEnabled 报告「容量过载时重抽会话身份」是否生效。
// 默认开启；配置 DisableCodexOverloadIdentityRotation 可整体回滚。
func (s *OpenAIGatewayService) CodexOverloadIdentityRotationEnabled() bool {
	return s != nil && s.cfg != nil && !s.cfg.Gateway.DisableCodexOverloadIdentityRotation
}

// pruneOpenAICodexOverloadBurstLocked 清理已过期条目；调用方必须持有锁。
// 若清理后仍超限（窗口内过载账号极多），整体重置：爆发追踪只是启发式预算提示，
// 丢失历史不会造成正确性问题，有界内存比保留旧窗口更重要。
func pruneOpenAICodexOverloadBurstLocked(now time.Time) {
	for id, st := range openAICodexOverloadBurst.state {
		if now.Sub(st.lastSeen) > openAICodexOverloadBurstWindow {
			delete(openAICodexOverloadBurst.state, id)
		}
	}
	if len(openAICodexOverloadBurst.state) < openAICodexOverloadBurstMaxEntries {
		return
	}
	logger.LegacyPrintf("service.openai_codex_overload",
		"overload burst tracker at capacity entries=%d, resetting", len(openAICodexOverloadBurst.state))
	openAICodexOverloadBurst.state = make(map[int64]codexOverloadBurstState, 16)
}
