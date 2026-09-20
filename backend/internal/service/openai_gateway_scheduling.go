package service

// 本文件由 openai_gateway_service.go 纯移动拆分而来：粘性会话哈希、账号选择、
// 负载感知调度与并发槽位获取。

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ExtractSessionID extracts the raw session ID from headers or body without hashing.
// Used by ForwardAsAnthropic to pass as prompt_cache_key for upstream cache.
func (s *OpenAIGatewayService) ExtractSessionID(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}
	sessionID := strings.TrimSpace(c.GetHeader("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.GetHeader("conversation_id"))
	}
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	return sessionID
}

func explicitOpenAISessionID(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}

	sessionID := strings.TrimSpace(c.GetHeader("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.GetHeader("conversation_id"))
	}
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	return sessionID
}

// GenerateExplicitSessionHash generates a sticky-session hash only from explicit
// client session signals. It intentionally skips content-derived fallback and is
// used by stateless endpoints such as /v1/images.
func (s *OpenAIGatewayService) GenerateExplicitSessionHash(c *gin.Context, body []byte) string {
	sessionID := explicitOpenAISessionID(c, body)
	if sessionID == "" {
		return ""
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(sessionID)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}

// GenerateSessionHash generates a sticky-session hash for OpenAI requests.
//
// Priority:
//  1. Header: session_id
//  2. Header: conversation_id
//  3. Body:   prompt_cache_key (opencode)
//  4. Body:   content-based fallback (model + system + tools + first user message)
func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}

	sessionID := explicitOpenAISessionID(c, body)
	if sessionID == "" && len(body) > 0 {
		sessionID = deriveOpenAIContentSessionSeed(body)
	}
	if sessionID == "" {
		return ""
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(sessionID)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}

// GenerateSessionHashWithFallback 先按常规信号生成会话哈希；
// 当未携带 session_id/conversation_id/prompt_cache_key 时，使用 fallbackSeed 生成稳定哈希。
// 该方法用于 WS ingress，避免会话信号缺失时发生跨账号漂移。
func (s *OpenAIGatewayService) GenerateSessionHashWithFallback(c *gin.Context, body []byte, fallbackSeed string) string {
	sessionHash := s.GenerateSessionHash(c, body)
	if sessionHash != "" {
		return sessionHash
	}

	seed := strings.TrimSpace(fallbackSeed)
	if seed == "" {
		return ""
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(seed)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}

func resolveOpenAIUpstreamOriginator(c *gin.Context, isOfficialClient bool) string {
	if c != nil {
		if originator := strings.TrimSpace(c.GetHeader("originator")); originator != "" {
			return originator
		}
	}
	if isOfficialClient {
		return "codex_cli_rs"
	}
	return "opencode"
}

// BindStickySession sets session -> account binding with standard TTL.
func (s *OpenAIGatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 {
		return nil
	}
	ttl := openaiStickySessionTTL
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	if err := s.setStickySessionAccountID(ctx, groupID, sessionHash, accountID, ttl); err != nil {
		return err
	}
	if account, err := s.getSchedulableAccount(ctx, accountID); err == nil && account != nil {
		s.setStickySessionAccountEpoch(ctx, groupID, sessionHash, account, ttl)
	}
	return nil
}

// SelectAccount selects an OpenAI account with sticky session support
func (s *OpenAIGatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel selects an account supporting the requested model
func (s *OpenAIGatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
// SelectAccountForModelWithExclusions 选择支持指定模型的账号，同时排除指定的账号。
func (s *OpenAIGatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	selector := s.accountSelector
	if selector == nil {
		selector = NewOpenAIAccountSelector(s)
	}
	selection, err := AccountSelectionFromScheduling(selector.Select(ctx, scheduling.Request{
		Platform:       PlatformOpenAI,
		GroupID:        groupID,
		StickyKey:      sessionHash,
		RequestedModel: requestedModel,
		ExcludedIDs:    excludedIDs,
		SkipAcquire:    true,
	}))
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil {
		return nil, ErrNoAvailableAccounts
	}
	if selection.ReleaseFunc != nil {
		selection.Release()
	}
	if openAICodexTurnStateReuseEnabled(s.cfg) &&
		!accountHasOpenAICodexTurnStateReserve(selection.Account, time.Now(), openAICodexTurnStateReuseTTL(s.cfg)) {
		s.scheduleOpenAICodexTurnStateRefresh(selection.Account)
	}
	return selection.Account, nil
}

// noAvailableOpenAISelectionError builds the standard "no account available" error
// while preserving the compact-specific error when applicable.
func normalizeOpenAICompatiblePlatform(platform string) string {
	switch platform {
	case PlatformGrok, PlatformKimi:
		return platform
	}
	return PlatformOpenAI
}

type noAvailableOpenAIAccountError struct {
	requestedModel string
	// reason 区分无候选层级：platform_empty（平台级候选池为空）或
	// model_filtered（平台级有候选但均被模型/能力/冷却刷掉）。
	reason string
}

func (e *noAvailableOpenAIAccountError) Error() string {
	if e == nil || e.requestedModel == "" {
		return "no available OpenAI accounts"
	}
	if e.reason != "" {
		return fmt.Sprintf("no available OpenAI accounts supporting model: %s (reason=%s)", e.requestedModel, e.reason)
	}
	return fmt.Sprintf("no available OpenAI accounts supporting model: %s", e.requestedModel)
}

func (e *noAvailableOpenAIAccountError) Unwrap() error {
	return ErrNoAvailableAccounts
}

func noAvailableOpenAISelectionError(requestedModel string, compactBlocked bool) error {
	if compactBlocked {
		return ErrNoAvailableCompactAccounts
	}
	return &noAvailableOpenAIAccountError{requestedModel: requestedModel}
}

// noAvailableOpenAISelectionErrorWithReason 与上者相同，但带缺失层级原因。
func noAvailableOpenAISelectionErrorWithReason(requestedModel, reason string, compactBlocked bool) error {
	if compactBlocked {
		return ErrNoAvailableCompactAccounts
	}
	return &noAvailableOpenAIAccountError{requestedModel: requestedModel, reason: reason}
}

// openAICompactSupportTier classifies an OpenAI account by compact capability.
// 0 = explicitly unsupported, 1 = unknown / not yet probed, 2 = explicitly supported.
func openAICompactSupportTier(account *Account) int {
	if account == nil || !account.IsOpenAI() {
		return 0
	}
	supported, known := account.OpenAICompactSupportKnown()
	if !known {
		return 1
	}
	if supported {
		return 2
	}
	return 0
}

var (
	openAICompatiblePolicy = scheduling.NewCandidatePolicy("openai_compatible", func(_ context.Context, account *Account, _ scheduling.Request) bool {
		return account != nil && account.IsOpenAICompatible()
	})
	providerQuotaPolicy = scheduling.NewCandidatePolicy("provider_quota_cooldown", func(_ context.Context, account *Account, _ scheduling.Request) bool {
		if account == nil {
			return false
		}
		if account.IsGrok() {
			if paused, reason := shouldAutoPauseGrokAccountByQuota(account); paused {
				slog.Debug("grok_account_auto_paused_by_quota",
					"account_id", account.ID,
					"window", reason.window,
					"threshold", reason.threshold,
					"utilization", reason.utilization,
				)
				return false
			}
		}
		return true
	})
	openAIRequestedModelPolicy = scheduling.NewCandidatePolicy("requested_model", func(_ context.Context, account *Account, req scheduling.Request) bool {
		return account != nil && (req.RequestedModel == "" || account.IsModelSupported(req.RequestedModel))
	})
	openAIEndpointCapabilityPolicy = scheduling.NewCandidatePolicy("endpoint_capability", func(_ context.Context, account *Account, req scheduling.Request) bool {
		return account != nil && account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapability(req.RequiredCapability))
	})
	openAICompactPolicy = scheduling.NewCandidatePolicy("compact", func(_ context.Context, account *Account, req scheduling.Request) bool {
		return account != nil && (!req.RequireCompact || (account.IsOpenAI() && openAICompactSupportTier(account) > 0))
	})
)

// isOpenAICompatibleAccountEligibleForRequest 判断 OpenAI 兼容账号是否满足本次请求的调度条件。
// 检查内容包括：平台匹配、账号可用性、quota 自动暂停、spark 路由限制、模型支持及端点能力。
//
// 注意：对 spark 影子账号，调用方还须额外调用 parentHealthyForShadow(account, lookup)
// 检查母账号凭据可用性；该检查未内置于本函数，以避免注入 DB 依赖。
func isOpenAICompatibleAccountEligibleForRequest(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) bool {
	platform = normalizeOpenAICompatiblePlatform(platform)
	return scheduling.EvaluateCandidate(
		ctx,
		account,
		scheduling.Request{
			Platform:           platform,
			RequestedModel:     requestedModel,
			RequireCompact:     requireCompact,
			RequiredCapability: string(requiredCapability),
		},
		scheduling.PlatformPolicy,
		scheduling.SchedulableForModelPolicy,
		openAICompatiblePolicy,
		providerQuotaPolicy,
		openAIRequestedModelPolicy,
		openAIEndpointCapabilityPolicy,
		openAICompactPolicy,
	).Allowed
}

type grokQuotaPauseDecision struct {
	window      string
	threshold   float64
	utilization float64
}

func shouldAutoPauseGrokAccountByQuota(account *Account) (bool, grokQuotaPauseDecision) {
	if account == nil || !account.IsGrok() || account.Type != AccountTypeOAuth {
		return false, grokQuotaPauseDecision{}
	}
	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		return false, grokQuotaPauseDecision{}
	}
	now := time.Now()
	if grokQuotaSnapshotStaleForPause(snapshot, now) {
		return false, grokQuotaPauseDecision{}
	}
	if grokQuotaRetryAfterActive(snapshot, now) {
		return true, grokQuotaPauseDecision{window: "retry_after", threshold: 1, utilization: 1}
	}
	if paused, decision := shouldAutoPauseGrokQuotaWindow("requests", snapshot.Requests, now); paused {
		return true, decision
	}
	if paused, decision := shouldAutoPauseGrokQuotaWindow("tokens", snapshot.Tokens, now); paused {
		return true, decision
	}
	return false, grokQuotaPauseDecision{}
}

func grokQuotaRetryAfterActive(snapshot *xai.QuotaSnapshot, now time.Time) bool {
	if snapshot == nil || snapshot.RetryAfterSeconds == nil || *snapshot.RetryAfterSeconds <= 0 {
		return false
	}
	if strings.TrimSpace(snapshot.UpdatedAt) == "" {
		return true
	}
	updatedAt, err := parseTime(snapshot.UpdatedAt)
	if err != nil {
		return true
	}
	retryAfterUntil := updatedAt.Add(time.Duration(*snapshot.RetryAfterSeconds) * time.Second)
	return now.Before(retryAfterUntil)
}

func shouldAutoPauseGrokQuotaWindow(name string, window *xai.QuotaWindow, now time.Time) (bool, grokQuotaPauseDecision) {
	if window == nil || window.Limit == nil || window.Remaining == nil || *window.Limit <= 0 {
		return false, grokQuotaPauseDecision{}
	}
	if window.ResetUnix != nil && *window.ResetUnix > 0 && !now.Before(time.Unix(*window.ResetUnix, 0)) {
		return false, grokQuotaPauseDecision{}
	}
	utilization := float64(*window.Limit-*window.Remaining) / float64(*window.Limit)
	if *window.Remaining <= 0 || utilization >= 1 {
		return true, grokQuotaPauseDecision{window: name, threshold: 1, utilization: utilization}
	}
	return false, grokQuotaPauseDecision{}
}

func grokQuotaSnapshotStaleForPause(snapshot *xai.QuotaSnapshot, now time.Time) bool {
	if snapshot == nil || strings.TrimSpace(snapshot.UpdatedAt) == "" {
		return false
	}
	updatedAt, err := parseTime(snapshot.UpdatedAt)
	if err != nil {
		return false
	}
	return now.Sub(updatedAt) >= grokQuotaPauseStaleAfter
}

// prioritizeOpenAICompactAccounts re-orders a slice so that accounts with known
// compact support are tried first, followed by unknown, then explicitly unsupported.
// The relative order within each tier is preserved.
func prioritizeOpenAICompactAccounts(accounts []*Account) []*Account {
	if len(accounts) == 0 {
		return nil
	}
	supported := make([]*Account, 0, len(accounts))
	unknown := make([]*Account, 0, len(accounts))
	unsupported := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		switch openAICompactSupportTier(account) {
		case 2:
			supported = append(supported, account)
		case 1:
			unknown = append(unknown, account)
		default:
			unsupported = append(unsupported, account)
		}
	}
	out := make([]*Account, 0, len(accounts))
	out = append(out, supported...)
	out = append(out, unknown...)
	out = append(out, unsupported...)
	return out
}

// resolveOpenAIAccountUpstreamModelForRequest resolves the upstream model that
// would be sent for a given request, honouring compact-only mappings when the
// caller is on the /responses/compact path.
func resolveOpenAIAccountUpstreamModelForRequest(account *Account, requestedModel string, requireCompact bool) string {
	upstreamModel := resolveOpenAIForwardModel(account, requestedModel, "")
	if upstreamModel == "" {
		return ""
	}
	if requireCompact {
		return resolveOpenAICompactForwardModel(account, upstreamModel)
	}
	return upstreamModel
}

// SelectAccountWithLoadAwareness selects an account with load-awareness and wait plan.
func (s *OpenAIGatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*AccountSelectionResult, error) {
	return s.selectAccountWithLoadAwareness(ctx, groupID, PlatformOpenAI, sessionHash, requestedModel, excludedIDs, false, "")
}

func (s *OpenAIGatewayService) selectAccountWithLoadAwareness(ctx context.Context, groupID *int64, platform string, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, requireCompact bool, requiredCapability OpenAIEndpointCapability) (*AccountSelectionResult, error) {
	selection, _, err := s.selectAccountThroughEngine(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportAny, requiredCapability, "", requireCompact, platform, false)
	return selection, err
}

func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string) ([]Account, error) {
	platform = normalizeOpenAICompatiblePlatform(platform)
	var snapshot scheduling.AccountListSnapshot
	if s.schedulerSnapshot != nil {
		snapshot = s.schedulerSnapshot
	}
	result, err := scheduling.ListAccounts(ctx, snapshot, s.accountRepo, scheduling.AccountListRequest{
		GroupID:       groupID,
		Platform:      platform,
		RepoPlatforms: []string{platform},
	})
	if err != nil {
		if result.FromSnapshot {
			return result.Accounts, err
		}
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	return result.Accounts, nil
}

func (s *OpenAIGatewayService) tryAcquireAccountSlot(ctx context.Context, requestedModel string, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	return tryAcquireAccountSlot(ctx, s.concurrencyService, requestedModel, accountID, maxConcurrency)
}

func (s *OpenAIGatewayService) resolveFreshSchedulableOpenAIAccount(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) *Account {
	if account == nil {
		return nil
	}
	platform = normalizeOpenAICompatiblePlatform(platform)

	fresh := account
	if s.schedulerSnapshot != nil {
		current, err := s.getSchedulableAccount(ctx, account.ID)
		if err != nil || current == nil {
			return nil
		}
		fresh = current
	}

	if !isOpenAICompatibleAccountEligibleForRequest(ctx, fresh, platform, requestedModel, requireCompact, requiredCapability) {
		return nil
	}
	if !parentHealthyForShadow(fresh, s.parentAccountLookup(ctx)) {
		return nil
	}
	if s.isOpenAIAccountRuntimeBlocked(fresh) {
		return nil
	}
	return fresh
}

// parentAccountLookup 返回供 parentHealthyForShadow 使用的母账号解析闭包:经 accountRepo
// 按 ID 取当前 Account(repo 为空时 fail-closed 返回 nil)。统一调度/粘连各路径的母账号解析,
// 取代各调用点重复内联的同一闭包(历史上 recheck 等路径还漏写过 accountRepo==nil 守卫)。
// L2 候选循环改用带 per-pass 缓存的 parentLookupL2,不走此方法。
// 请求级预取 map 存在时优先查 map,避免对母账号裸 GetByID。
func (s *OpenAIGatewayService) parentAccountLookup(ctx context.Context) func(int64) *Account {
	prefetch := accountPrefetchFromContext(ctx)
	return func(id int64) *Account {
		if s.accountRepo == nil {
			return nil
		}
		if prefetch != nil {
			if a, ok := prefetch[id]; ok {
				return a
			}
		}
		a, _ := s.accountRepo.GetByID(ctx, id)
		return a
	}
}

func (s *OpenAIGatewayService) recheckSelectedOpenAIAccountFromDB(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) *Account {
	if account == nil {
		return nil
	}
	platform = normalizeOpenAICompatiblePlatform(platform)
	if s.schedulerSnapshot == nil || s.accountRepo == nil {
		if !isOpenAICompatibleAccountEligibleForRequest(ctx, account, platform, requestedModel, requireCompact, requiredCapability) {
			return nil
		}
		if !parentHealthyForShadow(account, s.parentAccountLookup(ctx)) {
			return nil
		}
		return account
	}

	// 请求级预取 map 优先:调度入口已对全部候选(含 shadow 母账号)做一次性 GetByIDs,
	// 避免对每个候选账号逐个裸 GetByID(N+1)。map 缺失该 ID 时回退单查,语义与现状一致
	// (GetByID 返回 ErrAccountNotFound 时同样返回 nil)。
	var latest *Account
	if prefetch := accountPrefetchFromContext(ctx); prefetch != nil {
		latest = prefetch[account.ID]
		if latest == nil {
			return nil
		}
	} else {
		var err error
		latest, err = s.accountRepo.GetByID(ctx, account.ID)
		if err != nil || latest == nil {
			return nil
		}
	}
	if !isOpenAICompatibleAccountEligibleForRequest(ctx, latest, platform, requestedModel, requireCompact, requiredCapability) {
		return nil
	}
	if !parentHealthyForShadow(latest, s.parentAccountLookup(ctx)) {
		return nil
	}
	if s.isOpenAIAccountRuntimeBlocked(latest) {
		return nil
	}
	return latest
}

// withAccountPrefetch 将请求级账号预取 map 写入 context。map 为 nil 时同样写入,
// 以区分「已尝试预取但未命中」与「未预取」——后者回退逐账号查询。
func withAccountPrefetch(ctx context.Context, prefetch map[int64]*Account) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.AccountPrefetch, prefetch)
}

// accountPrefetchFromContext 读取请求级账号预取 map;未设置(或已设置为 nil)返回 nil。
func accountPrefetchFromContext(ctx context.Context) map[int64]*Account {
	if ctx == nil {
		return nil
	}
	prefetch, _ := ctx.Value(ctxkey.AccountPrefetch).(map[int64]*Account)
	return prefetch
}

// accountBatchRepo 可选接口:实现 GetByIDs 的仓库才启用批量预取。
// 测试桩大多只实现 GetByID 而未实现 GetByIDs(嵌入接口为 nil),直接调用会 panic;
// 通过 type-assert 让不支持批量读取的仓库自动跳过预取、回退逐账号查询,行为与未优化前一致。
type accountBatchRepo interface {
	GetByIDs(ctx context.Context, ids []int64) ([]*Account, error)
}

// prefetchOpenAIAccountsForRequest 对候选账号(含 shadow 母账号)做一次性 GetByIDs 预取,
// 并以带预取 map 的 context 返回。recheck / parent 解析在 map 命中时不再裸 GetByID,
// 把循环内 N 次单查压成 1 次批量读。
// 仅在 schedulerSnapshot 可用(recheck 走 DB 分支)且仓库支持批量读取时执行;
// GetByIDs 失败仅记日志,返回原 ctx,各调用点回退逐账号查询,行为与未优化前一致(fail-open)。
func (s *OpenAIGatewayService) prefetchOpenAIAccountsForRequest(ctx context.Context, candidates []*Account) context.Context {
	if ctx == nil {
		return ctx
	}
	if s.schedulerSnapshot == nil || s.accountRepo == nil || len(candidates) == 0 {
		return ctx
	}
	batchRepo, ok := s.accountRepo.(accountBatchRepo)
	if !ok {
		return ctx
	}
	ids := make([]int64, 0, len(candidates)*2)
	seen := make(map[int64]struct{}, len(candidates)*2)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, acc := range candidates {
		if acc == nil {
			continue
		}
		add(acc.ID)
		if acc.ParentAccountID != nil {
			add(*acc.ParentAccountID)
		}
	}
	if len(ids) == 0 {
		return ctx
	}
	accounts, err := batchRepo.GetByIDs(ctx, ids)
	if err != nil {
		slog.Warn("openai.account_prefetch_get_by_ids_failed", "count", len(ids), "error", err)
		return ctx
	}
	prefetch := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		if accounts[i] == nil {
			continue
		}
		prefetch[accounts[i].ID] = accounts[i]
	}
	return withAccountPrefetch(ctx, prefetch)
}

func (s *OpenAIGatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	return lookupSchedulableAccount(ctx, s.schedulerSnapshot, s.accountRepo, accountID)
}

func (s *OpenAIGatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	return newHydratedSelectionResult(ctx, s.schedulerSnapshot, account, "openai", acquired, release, waitPlan)
}

// newModelLimitedSelectionResult 构造模型级预算已满的拒绝结果。
// WaitPlan 恒为 nil，Handler 层据此直接 429 让路，不进入账号等待队列。
func (s *OpenAIGatewayService) newModelLimitedSelectionResult(ctx context.Context, account *Account) (*AccountSelectionResult, error) {
	return newHydratedModelLimitedResult(ctx, s.schedulerSnapshot, account, "openai")
}

func (s *OpenAIGatewayService) newAcquiredSelectionResult(ctx context.Context, account *Account, release func()) (*AccountSelectionResult, error) {
	return newHydratedAcquiredResult(ctx, s.schedulerSnapshot, account, "openai", release)
}

func (s *OpenAIGatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	return schedulingConfigFrom(s.cfg)
}
