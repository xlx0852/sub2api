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
	"github.com/tidwall/gjson"
)

const (
	openAICodexTurnStateHeader          = "x-codex-turn-state"
	openAICodexTurnStateExtraToken      = "openai_codex_turn_state"
	openAICodexTurnStateExtraModel      = "openai_codex_turn_state_model"
	openAICodexTurnStateExtraCapturedAt = "openai_codex_turn_state_captured_at"
	openAICodexTurnStateDefaultTTL      = 30 * time.Minute
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

func openAICodexTurnStateReuseTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Gateway.OpenAICodexTurnStateReuseTTLSeconds == 0 {
		return openAICodexTurnStateDefaultTTL
	}
	if cfg.Gateway.OpenAICodexTurnStateReuseTTLSeconds < 0 {
		return 0
	}
	return time.Duration(cfg.Gateway.OpenAICodexTurnStateReuseTTLSeconds) * time.Second
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
func lookupCachedOpenAICodexTurnStateForPool(account *Account, pool string, now time.Time, ttl time.Duration) string {
	if account == nil || account.Type != AccountTypeOAuth || account.Extra == nil {
		return ""
	}
	_ = now
	_ = ttl
	// Stale-while-revalidate: even expired tickets go out; seeder replaces them.
	if pool != "" {
		tokKey, _, _ := openAICodexTurnStatePoolKeys(pool)
		if token, _ := account.Extra[tokKey].(string); isReusableOpenAICodexTurnState(token) {
			return strings.TrimSpace(token)
		}
	}
	token, _ := account.Extra[openAICodexTurnStateExtraToken].(string)
	if isReusableOpenAICodexTurnState(token) {
		return strings.TrimSpace(token)
	}
	return ""
}

// lookupCachedOpenAICodexTurnStateForModel 按请求 model 的候选槽列表依次查票。
// astra 请求先试 astra 本名槽，再试 luna 槽，最后退回单值。
func lookupCachedOpenAICodexTurnStateForModel(account *Account, reqModel string, now time.Time, ttl time.Duration) string {
	if account == nil || account.Type != AccountTypeOAuth || account.Extra == nil {
		return ""
	}
	for _, pool := range openAICodexTurnStateCandidatePools(reqModel) {
		tokKey, _, _ := openAICodexTurnStatePoolKeys(pool)
		if tokKey == "" {
			continue
		}
		if token, _ := account.Extra[tokKey].(string); isReusableOpenAICodexTurnState(token) {
			return strings.TrimSpace(token)
		}
	}
	// 候选槽都空时退回兼容单值。
	token, _ := account.Extra[openAICodexTurnStateExtraToken].(string)
	if isReusableOpenAICodexTurnState(token) {
		return strings.TrimSpace(token)
	}
	return ""
}

func parseOpenAICodexTurnStateCapturedAt(account *Account) time.Time {
	if account == nil || account.Extra == nil {
		return time.Time{}
	}
	switch v := account.Extra[openAICodexTurnStateExtraCapturedAt].(type) {
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
		if lookupCachedOpenAICodexTurnState(account, time.Now(), openAICodexTurnStateReuseTTL(s.cfg)) != "" {
			s.touchOpenAICodexTurnState(account)
		}
		return
	}
	if pool == "" {
		pool = openAICodexTurnStatePoolForRequest(respModel)
	}
	s.storeOpenAICodexTurnStateForPool(account, pool, token, respModel)
}

func (s *OpenAIGatewayService) touchOpenAICodexTurnState(account *Account) {
	if account == nil {
		return
	}
	now := time.Now().UTC()
	openAICodexTurnStateMu.Lock()
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[openAICodexTurnStateExtraCapturedAt] = now.Format(time.RFC3339Nano)
	token, _ := account.Extra[openAICodexTurnStateExtraToken].(string)
	model, _ := account.Extra[openAICodexTurnStateExtraModel].(string)
	openAICodexTurnStateMu.Unlock()
	s.persistOpenAICodexTurnState(account.ID, token, model, now)
}

func (s *OpenAIGatewayService) storeOpenAICodexTurnState(account *Account, token, model string) {
	s.storeOpenAICodexTurnStateForPool(account, "", token, model)
}

// storeOpenAICodexTurnStateForPool 把票写入供应池槽位 + 兼容单值。
// pool 为空时用 openAICodexTurnStatePoolForRequest(model) 归槽。
// 单值 openai_codex_turn_state 仍保留为最后写入的一张（兼容旧读路径与兜底）。
func (s *OpenAIGatewayService) storeOpenAICodexTurnStateForPool(account *Account, pool, token, model string) {
	now := time.Now().UTC()
	if pool == "" {
		pool = openAICodexTurnStatePoolForRequest(model)
	}
	tokKey, modelKey, capturedKey := openAICodexTurnStatePoolKeys(pool)
	openAICodexTurnStateMu.Lock()
	if account.Extra == nil {
		account.Extra = map[string]any{}
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
	s.persistOpenAICodexTurnState(account.ID, token, model, now)
	if tokKey != "" {
		s.persistOpenAICodexTurnStatePool(account.ID, pool, token, model, now)
	}
}

func (s *OpenAIGatewayService) persistOpenAICodexTurnState(accountID int64, token, model string, now time.Time) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || token == "" {
		return
	}
	updates := map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraModel:      strings.TrimSpace(model),
		openAICodexTurnStateExtraCapturedAt: now.Format(time.RFC3339Nano),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(ctx, accountID, updates)
	}()
}

// persistOpenAICodexTurnStatePool 把池槽位三键落库。
func (s *OpenAIGatewayService) persistOpenAICodexTurnStatePool(accountID int64, pool, token, model string, now time.Time) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || token == "" {
		return
	}
	tokKey, modelKey, capturedKey := openAICodexTurnStatePoolKeys(pool)
	if tokKey == "" {
		return
	}
	updates := map[string]any{
		tokKey:      token,
		modelKey:    strings.TrimSpace(model),
		capturedKey: now.Format(time.RFC3339Nano),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(ctx, accountID, updates)
	}()
}

// openAICodexTurnStateRevokedSignals 是 312 降智信号的检测标记。
// 312 是 OpenAI 自定义的状态码（文章实测 2026-09），表示服务端主动撤销当前
// turn-state——即使票还在 TTL 内，后续请求已被降级（降智/overload）。
// 形态两种都可能：
//   1. HTTP 312 状态码（非标准，OpenAI 私有）
//   2. HTTP 200 流内 SSE 携带降级 marker（待确认字段，先用宽匹配）
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
	if account == nil || account.Type != AccountTypeOAuth {
		return
	}
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
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, clearUpdates)
	}()
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
	select {
	case ch <- account.ID:
	default:
	}
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
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return
	}
	ttl := openAICodexTurnStateReuseTTL(s.cfg)
	now := time.Now()
	need := make([]Account, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		if acc.Type != AccountTypeOAuth || acc.Status != StatusActive {
			continue
		}
		if onlyID > 0 && acc.ID != onlyID {
			continue
		}
		if !openAICodexTurnStateNeedsRefresh(&acc, now, ttl) && onlyID == 0 {
			continue
		}
		need = append(need, acc)
	}
	sortOpenAICodexTurnStateSeedQueue(need)
	for i := range need {
		s.seedOpenAICodexTurnState(ctx, &need[i])
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
	if account == nil || account.IsOpenAIAgentIdentity() {
		return
	}
	token := strings.TrimSpace(account.GetOpenAIAccessToken())
	chatgptID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if token == "" || chatgptID == "" {
		return
	}
	models := []string{"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol"}
	client := &http.Client{Timeout: 45 * time.Second}
	for _, model := range models {
		ok, hdr := probeOpenAICodexTurnState(ctx, client, token, chatgptID, model)
		if !ok {
			continue
		}
		s.captureOpenAICodexTurnState(account, hdr, model, true)
		if lookupCachedOpenAICodexTurnState(account, time.Now(), openAICodexTurnStateReuseTTL(s.cfg)) != "" {
			return
		}
	}
}

func probeOpenAICodexTurnState(ctx context.Context, client *http.Client, accessToken, chatgptAccountID, model string) (bool, http.Header) {
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
		return false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payload))
	if err != nil {
		return false, nil
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
		return false, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if !bytes.Contains(body, []byte(`"type":"response.completed"`)) && !bytes.Contains(body, []byte(`"type": "response.completed"`)) {
		return false, resp.Header
	}
	return true, resp.Header
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
