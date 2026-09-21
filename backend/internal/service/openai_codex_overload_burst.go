package service

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
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

// ---------------------------------------------------------------------------
// 会话级过载 epoch
// ---------------------------------------------------------------------------
//
// 为什么需要它：thread-id 是在「单个 HTTP 请求」的 gin context 内按失败重试轮次派生的
// （见 BumpCodexFingerprintAttempt）。客户端跨请求重试时（同一个客户端 session-id 发
// 下一条请求），轮次归零，派生回**同一个** thread-id——正好又回到被钉死的路由。
//
// 实测（本地 20159 个 turn / 190 次过载）：同一会话标识上重试成功率 19.6%，
// 换成新的会话标识 65.7%（同 30 分钟窗口对照 3.36×）。因此需要在过载后让
// **同一客户端会话**拿到新的派生值，持续到爆发窗口结束。
//
// 代价：该客户端会话在窗口内失去 thread 亲和（丢 prompt cache）。这是刻意的权衡：
// 窗口内本来就在严重降载，而失败重试的代价（整轮重发）远大于一次 cache miss。
//
// 键为 (accountID, 客户端 session-id) 的摘要，不保存原始 session-id。
// epoch 单调递增，只在窗口内生效；窗口外返回 0，派生回到确定性值。
type codexOverloadSessionEpochState struct {
	epoch    int
	lastSeen time.Time
}

var openAICodexOverloadSessionEpochs struct {
	mu sync.Mutex
	// active 为活跃条目数，用于让「从未发生过过载」的常见路径免于加锁。
	active int64
	state  map[string]codexOverloadSessionEpochState
}

// openAICodexOverloadSessionKey 返回 (accountID, clientSessionID) 的定长键。
// 用摘要而非原文，避免在内存里保留客户端会话标识。
func openAICodexOverloadSessionKey(accountID int64, clientSessionID string) string {
	if accountID <= 0 || clientSessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(clientSessionID))
	return fmt.Sprintf("%d:%x", accountID, sum[:8])
}

// rotateOpenAICodexOverloadSessionEpoch 记录一次过载并推进该 (账号, 客户端会话)
// 的派生 epoch，返回新值（>=1）。
//
// 返回值只用于观测与测试；生效与否由 OpenAICodexOverloadSessionEpoch 按窗口判定。
func rotateOpenAICodexOverloadSessionEpoch(accountID int64, clientSessionID string) int {
	key := openAICodexOverloadSessionKey(accountID, clientSessionID)
	if key == "" {
		return 0
	}
	now := time.Now()
	openAICodexOverloadSessionEpochs.mu.Lock()
	defer openAICodexOverloadSessionEpochs.mu.Unlock()
	if openAICodexOverloadSessionEpochs.state == nil {
		openAICodexOverloadSessionEpochs.state = make(map[string]codexOverloadSessionEpochState, 16)
	}
	st, ok := openAICodexOverloadSessionEpochs.state[key]
	if !ok {
		if len(openAICodexOverloadSessionEpochs.state) >= openAICodexOverloadBurstMaxEntries {
			pruneOpenAICodexOverloadSessionEpochsLocked(now)
		}
		openAICodexOverloadSessionEpochs.active++
	}
	// epoch 单调递增，不随窗口过期回退到旧值：避免同一会话重新拿到用过的派生值。
	st.epoch++
	st.lastSeen = now
	openAICodexOverloadSessionEpochs.state[key] = st
	return st.epoch
}

// OpenAICodexOverloadSessionEpoch 返回该 (账号, 客户端会话) 当前生效的过载 epoch，
// 窗口外或从未过载时为 0。
//
// 常见路径（从未发生过过载）靠 active 计数免于加锁。
func openAICodexOverloadSessionEpoch(accountID int64, clientSessionID string) int {
	if atomic.LoadInt64(&openAICodexOverloadSessionEpochs.active) == 0 {
		return 0
	}
	key := openAICodexOverloadSessionKey(accountID, clientSessionID)
	if key == "" {
		return 0
	}
	now := time.Now()
	openAICodexOverloadSessionEpochs.mu.Lock()
	defer openAICodexOverloadSessionEpochs.mu.Unlock()
	st, ok := openAICodexOverloadSessionEpochs.state[key]
	if !ok || now.Sub(st.lastSeen) > openAICodexOverloadBurstWindow {
		return 0
	}
	return st.epoch
}

// pruneOpenAICodexOverloadSessionEpochsLocked 清理已过期条目；调用方必须持有锁。
// 若清理后仍超限（窗口内过载会话极多），整体重置：epoch 只是路由启发式，
// 丢失历史只意味着回到确定性派生，不影响正确性，有界内存更重要。
func pruneOpenAICodexOverloadSessionEpochsLocked(now time.Time) {
	for key, st := range openAICodexOverloadSessionEpochs.state {
		if now.Sub(st.lastSeen) > openAICodexOverloadBurstWindow {
			delete(openAICodexOverloadSessionEpochs.state, key)
		}
	}
	if len(openAICodexOverloadSessionEpochs.state) < openAICodexOverloadBurstMaxEntries {
		atomic.StoreInt64(&openAICodexOverloadSessionEpochs.active, int64(len(openAICodexOverloadSessionEpochs.state)))
		return
	}
	logger.LegacyPrintf("service.openai_codex_overload",
		"overload session epoch tracker at capacity entries=%d, resetting", len(openAICodexOverloadSessionEpochs.state))
	openAICodexOverloadSessionEpochs.state = make(map[string]codexOverloadSessionEpochState, 16)
	atomic.StoreInt64(&openAICodexOverloadSessionEpochs.active, 0)
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
func (s *OpenAIGatewayService) OpenAICodexOverloadRetryBudget(account *Account, base int, overloaded bool) int {
	if base < 0 {
		base = 0
	}
	if !overloaded || account == nil || account.ID <= 0 || !s.CodexOverloadIdentityRotationEnabledFor(account) {
		return base
	}
	if s.OpenAICodexOverloadBurstAttempt(account.ID) < 2 {
		// 首次过载不额外放宽：这是抽签而非趋势，基线预算已经覆盖。
		return base
	}
	return base + openAICodexOverloadBurstRetryBonus
}

// CodexOverloadIdentityRotationEnabled 报告全局「容量过载时重抽会话身份」是否生效。
// 默认开启；配置 disable_codex_overload_identity_rotation 可整体回滚。
// 账号级覆盖见 CodexOverloadIdentityRotationEnabledFor。
func (s *OpenAIGatewayService) CodexOverloadIdentityRotationEnabled() bool {
	return s != nil && s.cfg != nil && !s.cfg.Gateway.DisableCodexOverloadIdentityRotation
}

// CodexOverloadIdentityRotationEnabledFor 在全局开关基础上应用账号级覆盖。
// 账号 extra 未配置时等价于全局开关（零值安全）。
//
// 灰度用法：把实验组与对照组都放在同一指纹模式下（否则「改指纹模式」与
// 「开轮换」两个变量会混在一起），只用账号 extra 的
// codex_overload_identity_rotation=on/off 区分。
func (s *OpenAIGatewayService) CodexOverloadIdentityRotationEnabledFor(account *Account) bool {
	if enabled, ok := GetCodexOverloadRotationOverride(account); ok {
		return enabled
	}
	return s.CodexOverloadIdentityRotationEnabled()
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
