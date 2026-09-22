package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
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
	// 票与 cookie 同属一个会话：票命中就一并注入上游签发的会话 cookie。
	// 出口指纹不匹配或已过期时返回空串（见 codexTurnStateCookieHeaderValue）。
	if cookie := codexTurnStateCookieHeaderValue(account, now); cookie != "" {
		headers.Set("Cookie", cookie)
	}
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
	// 拿到可复用票 = 该轮被上游接受。**只有被接受的响应才能刷新 cookie**：
	// 失败/被拒的响应若也能刷新，一次失败就会把上一份可用会话冲掉。
	s.captureCodexTurnStateCookies(account, headers, time.Now().UTC())
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
		// cookie 也一并清：它固定的是上游后端（亲和），而失效的语义是
		// 「当前这个会话不要了」——过载后只换票不换后端，等于继续钉在已经饱和的后端上。
		delete(account.Extra, openAICodexTurnStateCookieExtra)
		delete(account.Extra, openAICodexTurnStateCookieEgressExtra)
		for _, k := range poolKeys {
			delete(account.Extra, k)
		}
	}
	openAICodexTurnStateMu.Unlock()
	if s == nil || s.accountRepo == nil || account.ID <= 0 {
		return
	}
	clearUpdates := map[string]any{
		openAICodexTurnStateExtraToken:        nil,
		openAICodexTurnStateExtraModel:        nil,
		openAICodexTurnStateExtraCapturedAt:   nil,
		openAICodexTurnStateCookieExtra:       nil,
		openAICodexTurnStateCookieEgressExtra: nil,
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

// openAICodexTurnStateAgeSummary 汇总账号当前可复用票的数量与年龄区间。
//
// 存在的理由：票龄是否会招致上游降载/撤销，只能靠“出事时被回放的票有多旧”
// 来判定——上游对过期票的处置（忽略还是降级）在响应里不可观测。因此这里把
// 票龄挂到已有的失效日志上，让降载信号与票龄可以事后关联。
//
// 只统计池槽位：它们才是真正被回放的票；兼容单值槽位仅在没有任何池票时兜底，
// 否则会把同一张票重复计数。
//
// 纯诊断：不参与任何决策，不修改 state。
func openAICodexTurnStateAgeSummary(account *Account, now time.Time) string {
	if account == nil || account.Extra == nil {
		return "tickets=none"
	}
	openAICodexTurnStateMu.Lock()
	defer openAICodexTurnStateMu.Unlock()
	count := 0
	var minAge, maxAge time.Duration
	record := func(tokenRaw, capturedRaw any) {
		token, _ := tokenRaw.(string)
		if strings.TrimSpace(token) == "" {
			return
		}
		captured := parseOpenAICodexTurnStateTimeValue(capturedRaw)
		if captured.IsZero() || captured.After(now) {
			return
		}
		age := now.Sub(captured)
		if count == 0 || age < minAge {
			minAge = age
		}
		if count == 0 || age > maxAge {
			maxAge = age
		}
		count++
	}
	for k, v := range account.Extra {
		if !strings.HasPrefix(k, openAICodexTurnStateExtraPoolPrefix) {
			continue
		}
		pool := strings.TrimPrefix(k, openAICodexTurnStateExtraPoolPrefix)
		if pool == "" || strings.HasSuffix(k, "_model") || strings.HasSuffix(k, "_captured_at") {
			continue
		}
		_, _, capturedKey := openAICodexTurnStatePoolKeys(pool)
		record(v, account.Extra[capturedKey])
	}
	if count == 0 {
		record(account.Extra[openAICodexTurnStateExtraToken], account.Extra[openAICodexTurnStateExtraCapturedAt])
	}
	if count == 0 {
		return "tickets=none"
	}
	return fmt.Sprintf("tickets=%d age_min_s=%.1f age_max_s=%.1f", count, minAge.Seconds(), maxAge.Seconds())
}

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
	logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] invalidate account=%d reason=consecutive_5xx threshold=%d %s",
		account.ID, openAICodexTurnStateUpstream5xxInvalidateThreshold, openAICodexTurnStateAgeSummary(account, time.Now()))
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
		"[CodexTurnStateSeed] invalidate account=%d reason=server_overloaded path=%s burst_attempt=%d session_epoch=%d %s",
		account.ID, overloadPath, burstAttempt, epoch, openAICodexTurnStateAgeSummary(account, time.Now()))
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
	s.seedOpenAICodexTurnStateWithClient(ctx, account, nil)
}

// seedOpenAICodexTurnStateWithClient 采票。
// client 仅用于单测注入传输层；生产传 nil，走 s.httpUpstream + 账号代理，
// 保证**采票出口与业务出口一致**。
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
		result := s.probeOpenAICodexTurnState(ctx, account, token, chatgptID, model, client)
		probeTicket := strings.TrimSpace(result.headers.Get(openAICodexTurnStateHeader))
		if !result.completed || !isReusableOpenAICodexTurnState(probeTicket) {
			logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] probe_no_ticket account=%d model=%s status=%d completed=%v egress=%s",
				account.ID, model, result.status, result.completed, codexTurnStateProbeEgressLabel(account, client))
			continue
		}
		pool := result.model
		if pool == "" {
			pool = model
		}
		s.captureOpenAICodexTurnStateForPool(account, result.headers, pool, pool, true)
		// len 记录票的**形态**。形态与落池并不一一对应（实测 292↔astra、312↔luna/terra、
		// 332 也出现过在 luna），所以两个都记：requested_model 看请求、pool 看真实落点、
		// len 看形态。三者放在同一行才能回答「请求 astra 到底有没有拿到 astra」。
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateSeed] seeded account=%d requested_model=%s pool=%s len=%d egress=%s cookies=%d",
			account.ID, model, pool, len(probeTicket), codexTurnStateProbeEgressLabel(account, client), codexTurnStateCookieCount(account, time.Now()))
	}
}

// codexTurnStateProbeEgressLabel 标明本次采票的出口类型，用于生产取证：
// 采票出口必须与业务出口一致（account-proxy），不能是服务器直连。
// **只输出标签，绝不输出代理地址本身**（代理 URL 可能含凭据）。
func codexTurnStateProbeEgressLabel(account *Account, testClient *http.Client) string {
	if testClient != nil {
		return "test-injected"
	}
	if account != nil && account.Proxy != nil {
		return "account-proxy"
	}
	return "direct"
}

type codexTurnStateProbeResult struct {
	completed bool
	status    int
	headers   http.Header
	model     string
}

// codexTurnStateProbeBetaFeatures 是真实客户端在 /responses 上恒定发送的 beta 特性串。
// 实测（2026-09-22 全量抓包，/responses 123 条）123/123 都是这个值。
const codexTurnStateProbeBetaFeatures = "prevent_idle_sleep,remote_compaction_v2"

// codexTurnStateProbeBody 构造采票请求体。
func codexTurnStateProbeBody(model string) []byte {
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
		return nil
	}
	return payload
}

// codexTurnStateProbeTurnMetadata 合成 x-codex-turn-metadata。
//
// 键集合与取值全部取自实测（2026-09-22 全量抓包 /responses，23 个键）：键名一个不造，
// 枚举值取该抓包里出现过的组合。探针是账号级后台行为，没有入站客户端可转发，
// 所以只能合成——但合成的依据是真实流量而不是猜测。
func codexTurnStateProbeTurnMetadata(account *Account, sessionID, installationID, turnID, windowID, model string) string {
	metadata := map[string]any{
		"installation_id":                installationID,
		"session_id":                     sessionID,
		"thread_id":                      sessionID,
		"agent_name":                     "/root",
		"turn_id":                        turnID,
		"window_id":                      windowID,
		"window_number":                  0,
		"context_window_id":              deriveStableUUIDv4(fmt.Sprintf("codex:probe-context-window:v1:%d", account.ID)),
		"request_kind":                   "turn",
		"root_turn_id":                   turnID,
		"thread_source":                  "user",
		"turn_trigger":                   "composer",
		"sandbox":                        "none",
		"sandbox_mode":                   "danger-full-access",
		"auto_review_enabled":            false,
		"node_repl_auto_review_required": false,
		"node_repl_disabled":             false,
		"turn_started_at_unix_ms":        time.Now().UnixMilli(),
		"analytics_enabled":              true,
		"reasoning_effort":               "high",
		"workspace_kind":                 "project",
	}
	// 真实客户端在元数据里放的是**它请求的模型**（抓包里 102/120 是 gpt-6-astra，
	// 而实际服务模型多是 luna），所以这里也放请求模型而不是服务模型。
	if model = strings.TrimSpace(model); model != "" {
		metadata["model"] = model
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(raw)
}

// applyCodexTurnStateProbeIdentity 把采票请求做成与业务出站**同形**，并补齐真实客户端必定发送的头。
//
// 依据：2026-09-22 mitm 全量抓包（/responses 123 条，App/TUI/exec 三入口一致）。真实客户端
// 每一轮都带 accept / originator / session-id / thread-id / user-agent / x-client-request-id /
// x-codex-beta-features / x-codex-routing-hint / x-codex-turn-metadata / x-codex-window-id /
// x-openai-internal-codex-responses-lite。
//
// 此前探针只发 6 个头，于是**同一个账号、同一张票**出现两种请求形态：
// 极简 CLI 探针去采票，全副身份的业务请求去回放。这里让探针补齐同一套。
//
// 取值全部来自既有账号级身份派生（与 device/session 模式同源），不新造身份体系。
// 实测 session-id == thread-id == x-client-request-id（123/123），故三者取同一个账号级稳定值。
func applyCodexTurnStateProbeIdentity(h http.Header, svc *OpenAIGatewayService, account *Account, accessToken, chatgptAccountID, model string) {
	if h == nil {
		return
	}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("ChatGPT-Account-ID", chatgptAccountID)
	h.Set("Accept", "text/event-stream")
	h.Set("Content-Type", "application/json")
	// 身份统一与业务出站共用同一入口：账号显式 UA 优先，否则账号派生 UA。
	// enforceHeadersWithUA 在 originator 为空时直接返回，故先落一个默认值再由它覆写。
	h.Set("originator", "codex_cli_rs")
	if svc != nil && svc.codexIdentity != nil {
		svc.codexIdentity.enforceHeadersWithUA(h, svc.codexIdentityCandidateUA(nil, account))
	} else {
		h.Set("User-Agent", codexCLIUserAgent)
	}
	if account == nil {
		return
	}
	sessionID := resolveConvergedSessionID(account)
	if sessionID == "" {
		return
	}
	turnID := uuid.Must(uuid.NewV7()).String()
	windowID := sessionID + ":0"
	h.Set("session-id", sessionID)
	h.Set("thread-id", sessionID)
	h.Set("x-client-request-id", sessionID)
	h.Set("x-codex-window-id", windowID)
	h.Set("x-codex-beta-features", codexTurnStateProbeBetaFeatures)
	// 采票与业务共用同一份会话 cookie：票由探针采、由业务回放，
	// 两边带不同的亲和状态就等于又造了两种会话形态。
	if cookie := codexTurnStateCookieHeaderValue(account, time.Now()); cookie != "" {
		h.Set("Cookie", cookie)
	}
	if model = strings.TrimSpace(model); model != "" {
		// 与业务出站同一规则：取**出站 body 的 model**，保证头与体自洽。
		h.Set("x-codex-routing-hint", "model="+model)
	}
	// 这里**不发** x-openai-internal-codex-responses-lite：该头是「本体是 Lite 形态」的声明，
	// 而探针体是标准 Responses 形态（tools/include/instructions）。实测（2026-09-22）：
	// 给标准体加这个头，上游直接回 HTTP 400（旧探针基线里没有 400，只有 200 completed=false）。
	// 本仓已有同类注释：lite 头必须与体形状配套，否则 ChatGPT 拒绝。
	// 真实客户端发它是因为**它的体就是 Lite 形态**，不是「所有请求都该带它」。
	if metadata := codexTurnStateProbeTurnMetadata(account, sessionID, resolveConvergedInstallationID(account), turnID, windowID, model); metadata != "" {
		h.Set("x-codex-turn-metadata", metadata)
	}
}

// codexTurnStateProbeTimeout 采票单次探测的超时。
// 改走 httpUpstream 后默认响应头超时是 300s，且会占用账号并发额度；
// 后台探针不能因此长时间挂钩，保持原先裸 client 的 45s 边界。
const codexTurnStateProbeTimeout = 45 * time.Second

// probeOpenAICodexTurnState 发出一次采票请求。
//
// 出口：默认走 s.httpUpstream + **账号代理**，与业务请求同一出口。
// 此前探针用裸 http.Client 直连服务器 IP，而有代理的账号其业务流量走代理——
// 于是采票与回放在两个 IP 上发生（同一个会话/同一张票跨 IP），这是一个真实不一致。
// client 非 nil 时走注入的传输层，仅供单测使用。
func (s *OpenAIGatewayService) probeOpenAICodexTurnState(ctx context.Context, account *Account, accessToken, chatgptAccountID, model string, client *http.Client) codexTurnStateProbeResult {
	payload := codexTurnStateProbeBody(model)
	if payload == nil {
		return codexTurnStateProbeResult{}
	}
	ctx, cancel := context.WithTimeout(ctx, codexTurnStateProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payload))
	if err != nil {
		return codexTurnStateProbeResult{}
	}
	// 与业务出站同一个传输 profile：否则两者虽然都经 httpUpstream，
	// 实际走的却是不同传输策略，等于换一个维度重复同一个问题。
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	applyCodexTurnStateProbeIdentity(req.Header, s, account, accessToken, chatgptAccountID, model)

	// 与业务出站同形：OAuth 账号的 /responses 请求体会 zstd 压缩并声明 content-encoding。
	// 探针此前发的是未压缩体，于是同一个账号又是两种形态。上游接受 zstd 已在生产实测。
	if compressed, enc := compressOpenAIUpstreamBodyZstd(payload); enc != "" {
		req.Body = io.NopCloser(bytes.NewReader(compressed))
		req.ContentLength = int64(len(compressed))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(compressed)), nil
		}
		req.Header.Set(openAIRequestCompressionHeader, enc)
	}

	var resp *http.Response
	if client != nil {
		resp, err = client.Do(req)
	} else if s != nil && s.httpUpstream != nil {
		proxyURL := ""
		if account != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
		var accountID int64
		var concurrency int
		if account != nil {
			accountID, concurrency = account.ID, account.Concurrency
		}
		resp, err = s.httpUpstream.Do(req, proxyURL, accountID, concurrency)
	} else {
		return codexTurnStateProbeResult{}
	}
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
