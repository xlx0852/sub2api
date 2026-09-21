package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

const (
	openAICodexTurnStateHeader          = "x-codex-turn-state"
	openAICodexTurnStateExtraToken      = "openai_codex_turn_state"
	openAICodexTurnStateExtraModel      = "openai_codex_turn_state_model"
	openAICodexTurnStateExtraCapturedAt = "openai_codex_turn_state_captured_at"
	openAICodexTurnStateDefaultTTL      = 60 * time.Minute
	openAICodexTurnStateMinLen          = 200
	openAICodexTurnStateMaxLen          = 400
)

// openAICodexTurnStateExtraPoolPrefix 是按供应池分票的 extra key 前缀：
// openai_codex_turn_state_pool_<pool> = ticket
// openai_codex_turn_state_pool_<pool>_model = 签发时响应模型（resp_model）
// openai_codex_turn_state_pool_<pool>_captured_at = 捕获时间
//
// 供应池（pool）= 服务端实际执行模型（SSE response.model）。
// 实测（2026-09-18）：astra 别名与 luna 同一池，票互通；sol/terra 各自独立池，
// 跨池重签不认。故按 resp_model 归槽，请求时把 astra 映射到 luna 槽。
const openAICodexTurnStateExtraPoolPrefix = "openai_codex_turn_state_pool_"

// openAICodexTurnStatePoolForRequest 返回请求 slug 的主槽位名。
// 归槽以响应 resp_model 为准；这里只给出「请求该 model 时应优先查的槽」。
// astra 别名可能落本名池（resp_model=gpt-6-astra）或 luna 池（resp_model=gpt-5.6-luna），
// 两个槽位都会尝试，见 openAICodexTurnStateCandidatePools。
func openAICodexTurnStatePoolForRequest(reqModel string) string {
	return strings.TrimSpace(reqModel)
}

// openAICodexTurnStateCandidatePools 按请求 model 返回票槽位查找顺序。
// 实测（2026-09-18）：池间互不通用——astra 本名票打 luna 重签，sol 票打 astra 重签。
// 但 astra 请求本身会随机落 astra 本名池或 luna 别名池（69 全本名，112 多数 luna），
// 故 astra 请求查两个槽：本名槽优先（更准的粘性），其次 luna 槽。
// 其他 model：只查自身槽。
func openAICodexTurnStateCandidatePools(reqModel string) []string {
	m := strings.TrimSpace(reqModel)
	switch m {
	case "gpt-6-astra":
		return []string{"gpt-6-astra", "gpt-5.6-luna"}
	case "codex-auto-review":
		// Completed probe responses on both OAuth accounts identify the luna pool.
		return []string{"codex-auto-review", "gpt-5.6-luna"}
	}
	if m == "" {
		return nil
	}
	return []string{m}
}

// openAICodexTurnStatePoolKeys 返回某槽位的三个 extra key。
func openAICodexTurnStatePoolKeys(pool string) (tokKey, modelKey, capturedKey string) {
	pool = strings.TrimSpace(pool)
	if pool == "" {
		return "", "", ""
	}
	return openAICodexTurnStateExtraPoolPrefix + pool,
		openAICodexTurnStateExtraPoolPrefix + pool + "_model",
		openAICodexTurnStateExtraPoolPrefix + pool + "_captured_at"
}

var openAICodexTurnStateMu sync.Mutex

func openAICodexTurnStateReuseEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.Gateway.OpenAICodexTurnStateReuseEnabled
}

// Ticket lifetime is fixed; legacy configuration must not disable or extend it.
func openAICodexTurnStateReuseTTL(_ *config.Config) time.Duration {
	return openAICodexTurnStateDefaultTTL
}

func isReusableOpenAICodexTurnState(token string) bool {
	token = strings.TrimSpace(token)
	if n := len(token); n < openAICodexTurnStateMinLen || n > openAICodexTurnStateMaxLen {
		return false
	}
	if !strings.HasPrefix(token, "gAAAAA") {
		return false
	}
	for _, r := range token {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func lookupCachedOpenAICodexTurnState(account *Account, now time.Time, ttl time.Duration) string {
	return lookupCachedOpenAICodexTurnStateForPool(account, "", now, ttl)
}

// lookupCachedOpenAICodexTurnStateForPool 返回某供应池槽位的票。
// pool 为空时退回兼容行为：优先读单值 openai_codex_turn_state。
//
// 票龄硬失效：捕获时间超过 ttl（默认 1h）的票不再返回给上游——
// 不再采用 stale-while-revalidate；过期票不能继续回放。
func lookupCachedOpenAICodexTurnStateForPool(account *Account, pool string, now time.Time, ttl time.Duration) string {
	if account == nil || account.Type != AccountTypeOAuth || account.Extra == nil {
		return ""
	}
	if pool != "" {
		tokKey, _, capturedKey := openAICodexTurnStatePoolKeys(pool)
		if token, _ := account.Extra[tokKey].(string); isReusableOpenAICodexTurnState(token) && !openAICodexTurnStateTicketExpired(account.Extra[capturedKey], now, ttl) {
			return strings.TrimSpace(token)
		}
		return ""
	}
	token, _ := account.Extra[openAICodexTurnStateExtraToken].(string)
	if isReusableOpenAICodexTurnState(token) && !openAICodexTurnStateTicketExpired(account.Extra[openAICodexTurnStateExtraCapturedAt], now, ttl) {
		return strings.TrimSpace(token)
	}
	return ""
}

// Missing/invalid/future capture times cannot establish a valid ticket age.
func openAICodexTurnStateTicketExpired(capturedRaw any, now time.Time, ttl time.Duration) bool {
	if ttl <= 0 || ttl > openAICodexTurnStateDefaultTTL {
		ttl = openAICodexTurnStateDefaultTTL
	}
	captured := parseOpenAICodexTurnStateTimeValue(capturedRaw)
	return captured.IsZero() || captured.After(now) || now.Sub(captured) >= ttl
}

// lookupCachedOpenAICodexTurnStateForModel 按请求 model 的候选槽列表依次查票。
// astra 请求先试 astra 本名槽，再试 luna 槽；单值兜底必须匹配供应池。
func lookupCachedOpenAICodexTurnStateForModel(account *Account, reqModel string, now time.Time, ttl time.Duration) string {
	if account == nil || account.Type != AccountTypeOAuth || account.Extra == nil {
		return ""
	}
	for _, pool := range openAICodexTurnStateCandidatePools(reqModel) {
		tokKey, _, capturedKey := openAICodexTurnStatePoolKeys(pool)
		if tokKey == "" {
			continue
		}
		if token, _ := account.Extra[tokKey].(string); isReusableOpenAICodexTurnState(token) && !openAICodexTurnStateTicketExpired(account.Extra[capturedKey], now, ttl) {
			return strings.TrimSpace(token)
		}
	}
	// Only reuse the legacy single slot when its recorded model matches a candidate pool.
	// A fresh astra ticket must not conceal an expired terra/sol slot.
	if strings.TrimSpace(reqModel) == "" {
		return lookupCachedOpenAICodexTurnState(account, now, ttl)
	}
	legacyModel, _ := account.Extra[openAICodexTurnStateExtraModel].(string)
	for _, pool := range openAICodexTurnStateCandidatePools(reqModel) {
		if legacyModel == pool {
			return lookupCachedOpenAICodexTurnState(account, now, ttl)
		}
	}
	return ""
}

func parseOpenAICodexTurnStateCapturedAt(account *Account) time.Time {
	if account == nil || account.Extra == nil {
		return time.Time{}
	}
	return parseOpenAICodexTurnStateTimeValue(account.Extra[openAICodexTurnStateExtraCapturedAt])
}

// parseOpenAICodexTurnStateTimeValue 解析捕获时间原始值（string/time.Time/unix）。
func parseOpenAICodexTurnStateTimeValue(raw any) time.Time {
	switch v := raw.(type) {
	case time.Time:
		return v
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			return parsed
		}
	case float64:
		return time.Unix(int64(v), 0)
	case int64:
		return time.Unix(v, 0)
	case int:
		return time.Unix(int64(v), 0)
	}
	return time.Time{}
}

func openAICodexTurnStateNeedsRefresh(account *Account, now time.Time, ttl time.Duration) bool {
	token := lookupCachedOpenAICodexTurnState(account, now, ttl)
	if token == "" {
		return true
	}
	if ttl <= 0 {
		return false
	}
	captured := parseOpenAICodexTurnStateCapturedAt(account)
	if captured.IsZero() {
		return true
	}
	return now.Sub(captured) > ttl*3/4
}

func (s *OpenAIGatewayService) applyOpenAICodexTurnStateReuse(headers http.Header, account *Account) {
	s.applyOpenAICodexTurnStateReuseForModel(headers, account, "")
}

// applyOpenAICodexTurnStateReuseForModel 按请求 model 映射到供应池槽位取票。
// reqModel 为空时退回单值兼容路径。
func (s *OpenAIGatewayService) applyOpenAICodexTurnStateReuseForModel(headers http.Header, account *Account, reqModel string) {
	if headers == nil || !openAICodexTurnStateReuseEnabled(s.cfg) {
		return
	}
	headers.Del(openAICodexTurnStateHeader)
	now := time.Now()
	ttl := openAICodexTurnStateReuseTTL(s.cfg)
	token := lookupCachedOpenAICodexTurnStateForModel(account, reqModel, now, ttl)
	if token == "" {
		s.scheduleOpenAICodexTurnStateRefresh(account)
		return
	}
	headers.Set(openAICodexTurnStateHeader, token)
	if openAICodexTurnStateNeedsRefresh(account, now, ttl) {
		s.scheduleOpenAICodexTurnStateRefresh(account)
	}
}

// captureOpenAICodexTurnState 在成功响应上捕获新签发的 turn-state。
// model 应传 SSE response.model（真实落点），而不是请求 slug：
// 请求 astra 落 luna 时票归 luna 槽；若上游把某 slug 路由到本名池，也会按实际落点归槽。
func (s *OpenAIGatewayService) captureOpenAICodexTurnState(account *Account, headers http.Header, model string, success bool) {
	s.captureOpenAICodexTurnStateForPool(account, headers, "", model, success)
}

// captureOpenAICodexTurnStateForPool 与 capture 同义，但 pool 显式给出时按该槽位存；
// pool 为空时以 respModel 归槽（再退回 openAICodexTurnStatePoolForRequest）。
func (s *OpenAIGatewayService) captureOpenAICodexTurnStateForPool(account *Account, headers http.Header, pool, respModel string, success bool) {
	if s == nil || !openAICodexTurnStateReuseEnabled(s.cfg) || !success || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	if headers == nil {
		return
	}
	token := strings.TrimSpace(headers.Get(openAICodexTurnStateHeader))
	if token == "" {
		token = strings.TrimSpace(headers.Get(openAIWSTurnStateHeader))
	}
	if !isReusableOpenAICodexTurnState(token) {
		return
	}
	if pool == "" {
		pool = openAICodexTurnStatePoolForRequest(respModel)
	}
	s.storeOpenAICodexTurnStateForPool(account, pool, token, respModel)
}

func (s *OpenAIGatewayService) storeOpenAICodexTurnState(account *Account, token, model string) {
	s.storeOpenAICodexTurnStateForPool(account, "", token, model)
}

// storeOpenAICodexTurnStateForPool 把票写入供应池槽位 + 兼容单值。
// pool 为空时用 openAICodexTurnStatePoolForRequest(model) 归槽。
// 单值 openai_codex_turn_state 仍保留为最后写入的一张（兼容旧读路径与兜底）。
func (s *OpenAIGatewayService) storeOpenAICodexTurnStateForPool(account *Account, pool, token, model string) {
	s.codexTurnStatePersistMu.Lock()
	defer s.codexTurnStatePersistMu.Unlock()
	now := time.Now().UTC()
	if pool == "" {
		pool = openAICodexTurnStatePoolForRequest(model)
	}
	tokKey, modelKey, capturedKey := openAICodexTurnStatePoolKeys(pool)
	openAICodexTurnStateMu.Lock()
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	// Repeated response headers do not create a new issuance time.
	if previous, _ := account.Extra[tokKey].(string); tokKey != "" && previous == token {
		openAICodexTurnStateMu.Unlock()
		return
	}
	// 池槽位
	if tokKey != "" {
		account.Extra[tokKey] = token
		account.Extra[modelKey] = strings.TrimSpace(model)
		account.Extra[capturedKey] = now.Format(time.RFC3339Nano)
	}
	// 兼容单值：始终跟随最新一张
	account.Extra[openAICodexTurnStateExtraToken] = token
	account.Extra[openAICodexTurnStateExtraModel] = strings.TrimSpace(model)
	account.Extra[openAICodexTurnStateExtraCapturedAt] = now.Format(time.RFC3339Nano)
	openAICodexTurnStateMu.Unlock()
	updates := map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraModel:      strings.TrimSpace(model),
		openAICodexTurnStateExtraCapturedAt: now.Format(time.RFC3339Nano),
	}
	if tokKey != "" {
		updates[tokKey], updates[modelKey], updates[capturedKey] = token, strings.TrimSpace(model), now.Format(time.RFC3339Nano)
	}
	s.persistOpenAICodexTurnStateUpdates(account.ID, updates)
}

// Complete persistence before a seeder job finishes or a subsequent invalidation runs.
func (s *OpenAIGatewayService) persistOpenAICodexTurnStateUpdates(accountID int64, updates map[string]any) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.accountRepo.UpdateExtra(ctx, accountID, updates); err != nil {
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] persist failed account=%d", accountID)
	}
}

// openAICodexTurnStateRevokedSignals 是 312 降智信号的检测标记。
// 312 是 OpenAI 自定义的状态码（文章实测 2026-09），表示服务端主动撤销当前
// turn-state——即使票还在 TTL 内，后续请求已被降级（降智/overload）。
// 形态两种都可能：
//  1. HTTP 312 状态码（非标准，OpenAI 私有）
//  2. HTTP 200 流内 SSE 携带降级 marker（待确认字段，先用宽匹配）
//
// 一旦命中即废弃该账号所有池槽位票并触发重采。
const openAICodexTurnStateRevokedHTTPStatus = 312

// isOpenAICodexTurnStateRevokedResponse 判断响应是否携带 312 降智信号。
// 检查 HTTP 状态码与 SSE payload 两处。
func isOpenAICodexTurnStateRevokedResponse(httpStatus int, payload []byte) bool {
	if httpStatus == openAICodexTurnStateRevokedHTTPStatus {
		return true
	}
	if len(payload) == 0 {
		return false
	}
	// 流内 marker：error.code / response.error.code / type 字段含 312 / degraded / downgraded
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	eventType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "type").String()))
	combined := strings.ToLower(strings.TrimSpace(code + " " + eventType))
	switch {
	case strings.Contains(combined, "312"),
		strings.Contains(combined, "degraded"),
		strings.Contains(combined, "downgraded"),
		strings.Contains(combined, "state_revoked"),
		strings.Contains(combined, "turn_state_revoked"):
		return true
	}
	return false
}

// invalidateOpenAICodexTurnState 废弃账号所有池槽位票并触发重采。
// 触发条件：收到 312 降智信号（票被服务端撤销）、或明确的容量降级。
func (s *OpenAIGatewayService) invalidateOpenAICodexTurnState(account *Account) {
	if s == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	s.codexTurnStatePersistMu.Lock()
	defer s.codexTurnStatePersistMu.Unlock()
	// 先收集要清理的池槽位 key（锁内），再统一清内存与落库。
	poolKeys := make([]string, 0, 8)
	openAICodexTurnStateMu.Lock()
	if account.Extra != nil {
		for k := range account.Extra {
			if strings.HasPrefix(k, openAICodexTurnStateExtraPoolPrefix) {
				poolKeys = append(poolKeys, k)
			}
		}
		delete(account.Extra, openAICodexTurnStateExtraToken)
		delete(account.Extra, openAICodexTurnStateExtraModel)
		delete(account.Extra, openAICodexTurnStateExtraCapturedAt)
		for _, k := range poolKeys {
			delete(account.Extra, k)
		}
	}
	openAICodexTurnStateMu.Unlock()
	if s == nil || s.accountRepo == nil || account.ID <= 0 {
		return
	}
	clearUpdates := map[string]any{
		openAICodexTurnStateExtraToken:      nil,
		openAICodexTurnStateExtraModel:      nil,
		openAICodexTurnStateExtraCapturedAt: nil,
	}
	for _, k := range poolKeys {
		clearUpdates[k] = nil
	}
	s.persistOpenAICodexTurnStateUpdates(account.ID, clearUpdates)
}

func (s *OpenAIGatewayService) scheduleOpenAICodexTurnStateRefresh(account *Account) {
	if s == nil || account == nil || account.Type != AccountTypeOAuth || account.ID <= 0 {
		return
	}
	if !openAICodexTurnStateReuseEnabled(s.cfg) {
		return
	}
	s.startOpenAICodexTurnStateSeeder()
	ch := s.codexTurnStateRefresh
	if ch == nil {
		return
	}
	if !s.claimOpenAICodexTurnStateRefresh(account.ID, time.Now()) {
		return
	}
	select {
	case ch <- account.ID:
	default:
		s.finishOpenAICodexTurnStateRefresh(account.ID, time.Now())
	}
}

type codexTurnStateRefreshState struct {
	pending bool
	next    time.Time
}

func (s *OpenAIGatewayService) claimOpenAICodexTurnStateRefresh(id int64, now time.Time) bool {
	s.codexTurnStateWorkMu.Lock()
	defer s.codexTurnStateWorkMu.Unlock()
	if s.codexTurnStateWork == nil {
		s.codexTurnStateWork = make(map[int64]codexTurnStateRefreshState)
	}
	for key, state := range s.codexTurnStateWork {
		if !state.pending && !now.Before(state.next) {
			delete(s.codexTurnStateWork, key)
		}
	}
	state := s.codexTurnStateWork[id]
	if state.pending || now.Before(state.next) {
		return false
	}
	s.codexTurnStateWork[id] = codexTurnStateRefreshState{pending: true}
	return true
}

func (s *OpenAIGatewayService) finishOpenAICodexTurnStateRefresh(id int64, now time.Time) {
	s.codexTurnStateWorkMu.Lock()
	defer s.codexTurnStateWorkMu.Unlock()
	s.codexTurnStateWork[id] = codexTurnStateRefreshState{next: now.Add(2 * time.Minute)}
}

// Each pool needs its own refresh deadline, independent of the last-issued single slot.
var openAICodexTurnStateSeedModels = []string{
	"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "codex-auto-review",
}

func openAICodexTurnStateModelsNeedingRefresh(account *Account, now time.Time, ttl time.Duration) []string {
	var models []string
	for _, model := range openAICodexTurnStateSeedModels {
		if account.IsModelSupported(model) && lookupCachedOpenAICodexTurnStateForModel(account, model, now, ttl*3/4) == "" {
			models = append(models, model)
		}
	}
	return models
}

// openAICodexTurnStateUpstream5xxInvalidateThreshold 同号连续上游 5xx 达到该次数时
// 认为其 turn-state 票可能已被服务端吊销/变质，主动清票重抓。
// 502 不是 312 降智信号，但观测到某号带票持续 502（而其他号同模型正常）——
// 这里只作为恢复启发式，不代表 5xx 已证实是票失效。
const openAICodexTurnStateUpstream5xxInvalidateThreshold = 3

// NoteOpenAICodexTurnStateUpstreamResult 记录该号一次上游结果：
//   - status>=500：累计连续 5xx；达到阈值 → invalidate 所有池票 + 触发重抓，并清零计数。
//   - status<500：成功/4xx，清零计数。
//
// 只对 oauth（Codex 上游）号生效。
// transportOnly=true 表示该次失败是传输层错误（代理/DNS/TCP/TLS），上游未返回
// 真实 HTTP 响应；这类错误既不是票失效也不是上游过载，不得计入连续 5xx，否则
// 会出现"代理断了却反复清票重抓"的误判。
func (s *OpenAIGatewayService) NoteOpenAICodexTurnStateUpstreamResult(account *Account, upstreamStatus int, transportOnly ...bool) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || account.ID <= 0 || !openAICodexTurnStateReuseEnabled(s.cfg) {
		return
	}
	if len(transportOnly) > 0 && transportOnly[0] {
		return
	}
	if !s.shouldInvalidateOpenAICodexTurnState(account.ID, upstreamStatus, time.Now()) {
		return
	}
	logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] invalidate account=%d reason=consecutive_5xx threshold=%d", account.ID, openAICodexTurnStateUpstream5xxInvalidateThreshold)
	s.invalidateOpenAICodexTurnState(account)
	s.scheduleOpenAICodexTurnStateRefresh(account)
}

// HandleOpenAICodexOverload 处理一次明确的容量过载（server_is_overloaded / slow_down）。
// 做三件事：记录爆发窗口、推进 (账号, 客户端会话) 的过载 epoch、丢弃被钉住的票。
//
// 为什么要清票（而非等连续 5xx 阈值）：票是路由提示，被钉住的票正是把重试锁在
// 同一个已饱和后端的原因；过载是瞬时抽签，第 1 次就应重抽。
// 为什么要 epoch：客户端跨请求重试时请求内轮次会归零，只有 epoch 能让下一条请求
// 拿到新的 thread-id（实测同会话重试 19.6% vs 换会话 65.7%）。
//
// 过载是请求级信号而非账号级故障：本入口只清票/推进 epoch，不动账号健康与调度状态。
func (s *OpenAIGatewayService) HandleOpenAICodexOverload(account *Account, overloadPath, clientSessionID string) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || account.ID <= 0 {
		return
	}
	if !s.CodexOverloadIdentityRotationEnabledFor(account) {
		return
	}
	// 爆发窗口与 epoch 记录与票据复用解耦：重试预算（OpenAICodexOverloadRetryBudget）
	// 与跨请求重抽依赖它们，即使账号没启用票据复用也应当记录。
	burstAttempt := s.OnOpenAICodexOverload(account.ID)
	epoch := rotateOpenAICodexOverloadSessionEpoch(account.ID, clientSessionID)
	if !openAICodexTurnStateReuseEnabled(s.cfg) {
		return
	}
	// 过载不代表票“被吊销”，但必须重抽：同时清零连续 5xx 计数，
	// 避免后续真实 5xx 因过载计数被提前触发连续阈值语义。
	s.codexTurnStateFailureMu.Lock()
	s.codexTurnStateUpstream5xx.Delete(account.ID)
	s.codexTurnStateFailureMu.Unlock()
	logger.LegacyPrintf("service.openai_codex_turn_state",
		"[CodexTurnStateSeed] invalidate account=%d reason=server_overloaded path=%s burst_attempt=%d session_epoch=%d",
		account.ID, overloadPath, burstAttempt, epoch)
	s.invalidateOpenAICodexTurnState(account)
	s.scheduleOpenAICodexTurnStateRefresh(account)
}

func (s *OpenAIGatewayService) shouldInvalidateOpenAICodexTurnState(accountID int64, upstreamStatus int, now time.Time) bool {
	s.codexTurnStateFailureMu.Lock()
	defer s.codexTurnStateFailureMu.Unlock()
	if upstreamStatus < 500 {
		s.codexTurnStateUpstream5xx.Delete(accountID)
		return false
	}
	raw, _ := s.codexTurnStateUpstream5xx.LoadOrStore(accountID, 0)
	count, _ := raw.(int)
	count++
	if count < openAICodexTurnStateUpstream5xxInvalidateThreshold {
		s.codexTurnStateUpstream5xx.Store(accountID, count)
		return false
	}
	s.codexTurnStateUpstream5xx.Delete(accountID)
	if s.codexTurnStateInvalidatedAt == nil {
		s.codexTurnStateInvalidatedAt = make(map[int64]time.Time)
	}
	if now.Sub(s.codexTurnStateInvalidatedAt[accountID]) < 2*time.Minute {
		return false
	}
	s.codexTurnStateInvalidatedAt[accountID] = now
	return true
}

func (s *OpenAIGatewayService) startOpenAICodexTurnStateSeeder() {
	if s == nil || s.accountRepo == nil || !openAICodexTurnStateReuseEnabled(s.cfg) {
		return
	}
	s.codexTurnStateSeedOnce.Do(func() {
		s.codexTurnStateRefresh = make(chan int64, 32)
		go s.runOpenAICodexTurnStateSeeder()
	})
}

func (s *OpenAIGatewayService) runOpenAICodexTurnStateSeeder() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	s.refreshOpenAICodexTurnStateAccounts(context.Background(), 0)
	for {
		select {
		case id := <-s.codexTurnStateRefresh:
			s.refreshOpenAICodexTurnStateAccounts(context.Background(), id)
		case <-ticker.C:
			s.refreshOpenAICodexTurnStateAccounts(context.Background(), 0)
		}
	}
}

func (s *OpenAIGatewayService) refreshOpenAICodexTurnStateAccounts(ctx context.Context, onlyID int64) {
	if s == nil || s.accountRepo == nil {
		return
	}
	if onlyID > 0 {
		defer func() { s.finishOpenAICodexTurnStateRefresh(onlyID, time.Now()) }()
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return
	}
	ttl := openAICodexTurnStateReuseTTL(s.cfg)
	now := time.Now()
	need := make([]Account, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		if acc.Type != AccountTypeOAuth || !acc.IsSchedulable() || acc.IsOpenAIAgentIdentity() {
			continue
		}
		if onlyID > 0 && acc.ID != onlyID {
			continue
		}
		if len(openAICodexTurnStateModelsNeedingRefresh(&acc, now, ttl)) == 0 {
			continue
		}
		need = append(need, acc)
	}
	sortOpenAICodexTurnStateSeedQueue(need)
	if len(need) > 0 {
		ids := make([]int64, 0, len(need))
		for i := range need {
			ids = append(ids, need[i].ID)
		}
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] refresh need=%d onlyID=%d ids=%v", len(need), onlyID, ids)
	}
	for i := range need {
		if onlyID == 0 && !s.claimOpenAICodexTurnStateRefresh(need[i].ID, time.Now()) {
			continue
		}
		s.seedOpenAICodexTurnState(ctx, &need[i])
		if onlyID == 0 {
			s.finishOpenAICodexTurnStateRefresh(need[i].ID, time.Now())
		}
		time.Sleep(400 * time.Millisecond)
	}
}

// sortOpenAICodexTurnStateSeedQueue 没票账号按入池时间 FIFO（CreatedAt，其次 ID）。
func sortOpenAICodexTurnStateSeedQueue(accounts []Account) {
	sort.SliceStable(accounts, func(i, j int) bool {
		ti, tj := accounts[i].CreatedAt, accounts[j].CreatedAt
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return accounts[i].ID < accounts[j].ID
	})
}

func accountHasOpenAICodexTurnStateReserve(account *Account, now time.Time, ttl time.Duration) bool {
	return lookupCachedOpenAICodexTurnState(account, now, ttl) != ""
}

// accountHasOpenAICodexTurnStateReserveForModel 按请求 model 判断任一候选池槽位是否有票。
// 用于调度：astra 请求认 astra 或 luna 槽的票；sol/terra 只认自身槽。
func accountHasOpenAICodexTurnStateReserveForModel(account *Account, reqModel string, now time.Time, ttl time.Duration) bool {
	return lookupCachedOpenAICodexTurnStateForModel(account, reqModel, now, ttl) != ""
}

func (s *OpenAIGatewayService) seedOpenAICodexTurnState(ctx context.Context, account *Account) {
	s.seedOpenAICodexTurnStateWithClient(ctx, account, &http.Client{Timeout: 45 * time.Second})
}

func (s *OpenAIGatewayService) seedOpenAICodexTurnStateWithClient(ctx context.Context, account *Account, client *http.Client) {
	if account == nil || account.IsOpenAIAgentIdentity() {
		return
	}
	token := strings.TrimSpace(account.GetOpenAIAccessToken())
	chatgptID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if token == "" || chatgptID == "" {
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] skip account=%d missing_token=%v missing_cid=%v", account.ID, token == "", chatgptID == "")
		return
	}
	models := openAICodexTurnStateModelsNeedingRefresh(account, time.Now(), openAICodexTurnStateReuseTTL(s.cfg))
	for _, model := range models {
		if lookupCachedOpenAICodexTurnStateForModel(account, model, time.Now(), openAICodexTurnStateReuseTTL(s.cfg)*3/4) != "" {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		result := probeOpenAICodexTurnState(ctx, client, token, chatgptID, model)
		if !result.completed || !isReusableOpenAICodexTurnState(result.headers.Get(openAICodexTurnStateHeader)) {
			logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] probe_no_ticket account=%d model=%s status=%d completed=%v", account.ID, model, result.status, result.completed)
			continue
		}
		pool := result.model
		if pool == "" {
			pool = model
		}
		s.captureOpenAICodexTurnStateForPool(account, result.headers, pool, pool, true)
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] seeded account=%d requested_model=%s pool=%s", account.ID, model, pool)
	}
}

type codexTurnStateProbeResult struct {
	completed bool
	status    int
	headers   http.Header
	model     string
}

func probeOpenAICodexTurnState(ctx context.Context, client *http.Client, accessToken, chatgptAccountID, model string) codexTurnStateProbeResult {
	payload, err := json.Marshal(map[string]any{
		"model":               model,
		"instructions":        "Reply with exactly OK. Do not use tools.",
		"input":               []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Say only: OK"}}}},
		"tools":               []any{},
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
		"store":               false,
		"stream":              true,
		"include":             []string{"reasoning.encrypted_content"},
	})
	if err != nil {
		return codexTurnStateProbeResult{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payload))
	if err != nil {
		return codexTurnStateProbeResult{}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("ChatGPT-Account-ID", chatgptAccountID)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("User-Agent", "codex_cli_rs/"+codexCLIVersion)
	req.Header.Set("version", codexCLIVersion)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return codexTurnStateProbeResult{}
	}
	defer resp.Body.Close()
	result := codexTurnStateProbeResult{status: resp.StatusCode, headers: resp.Header}
	if resp.StatusCode != http.StatusOK {
		return result
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return result
	}
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if gjson.GetBytes(data, "type").String() == "response.completed" {
			result.completed = true
			result.model = gjson.GetBytes(data, "response.model").String()
		}
	}
	return result
}

func (s *OpenAIGatewayService) fallbackOpenAICodexTurnState(account *Account, current string) string {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current)
	}
	if !openAICodexTurnStateReuseEnabled(s.cfg) {
		return ""
	}
	return lookupCachedOpenAICodexTurnState(account, time.Now(), openAICodexTurnStateReuseTTL(s.cfg))
}
