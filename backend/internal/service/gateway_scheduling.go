package service

// Gateway provider hard gates and affinity wrap the shared priority/load order.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

type gatewayPlatformHintContextKey struct{}

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 请求耗时详情：账号选择（Anthropic/Gemini 网关路径）。
	selectStarted := time.Now()
	defer func() {
		if collector := requesttiming.FromContext(ctx); collector != nil {
			collector.SetAccountSelectMs(time.Since(selectStarted))
		}
	}()
	selection, err := AccountSelectionFromScheduling(NewGatewayAccountSelector(s).Select(ctx, scheduling.Request{
		Platform:       PlatformAnthropic,
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
	return selection.Account, nil
}

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
// metadataUserID: 用于客户端亲和调度，从中提取客户端 ID
// sub2apiUserID: 系统用户 ID，用于二维亲和调度
//
// selectAccountWithLoadAwarenessStrategy 是网关选号编排入口：
// 单次尝试 + 空池时的强制 DB 复核重试（见 gateway_empty_pool_recheck.go）。
func (s *GatewayService) selectAccountWithLoadAwarenessStrategy(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	return s.selectGatewayAccountWithEmptyPoolRecheck(ctx, groupID, requestedModel, excludedIDs,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			return s.selectGatewayAccountOnce(ctx, groupID, sessionHash, requestedModel, excludedIDs, metadataUserID, sub2apiUserID)
		})
}

// selectGatewayAccountOnce 执行一次完整的网关选号，并回传本次尝试的池规模。
// 仅当返回 ErrNoAvailableAccounts 且 stats.EmptyPool 为 true 时，调用方才会做 DB 复核重试。
func (s *GatewayService) selectGatewayAccountOnce(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
	stats := gatewaySelectionPoolStats{}
	cfg := s.schedulingConfig()
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return nil, stats, err
	}
	ctx = s.withGroupContext(ctx, group)
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		return nil, stats, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}
	platform, force, err := s.resolvePlatform(ctx, groupID, group)
	if err != nil {
		return nil, stats, err
	}
	stats.Platform = platform
	stats.HasForcePlatform = force
	accounts, mixed, err := s.listSchedulableAccounts(ctx, groupID, platform, force)
	if err != nil {
		return nil, stats, err
	}
	stats.PoolSize = len(accounts)
	if len(accounts) == 0 {
		stats.EmptyPool = true
		return nil, stats, ErrNoAvailableAccounts
	}
	stickyID := prefetchedStickyAccountIDFromContext(ctx, groupID)
	if stickyID <= 0 && sessionHash != "" && s.cache != nil {
		stickyID, _ = s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
	}
	req := scheduling.Request{Platform: platform, GroupID: groupID, RequestedModel: requestedModel, ExcludedIDs: excludedIDs, AllowMixedPlatform: mixed}
	checkUpstream := false
	if groupID != nil && s.channelService != nil {
		ch, e := s.channelService.GetChannelForGroup(ctx, *groupID)
		checkUpstream = e == nil && ch != nil && ch.RestrictModels && ch.BillingModelSource == BillingModelSourceUpstream
	}
	policies := s.gatewayCandidatePoliciesFor(gatewayCandidatePolicyOptions{group: group, checkUpstreamChannel: checkUpstream})
	allowed := func(a *Account) bool {
		r := req
		r.Sticky = a != nil && a.ID == stickyID && !shouldClearStickySession(a, requestedModel)
		return scheduling.EvaluateCandidate(ctx, a, r, policies...).Allowed
	}
	candidates := make([]*Account, 0, len(accounts))
	trace := make([]scheduling.CandidatePolicyTrace, 0, len(accounts))
	validSticky := false
	for i := range accounts {
		a := &accounts[i]
		r := req
		r.Sticky = a.ID == stickyID && !shouldClearStickySession(a, requestedModel)
		d := scheduling.EvaluateCandidate(ctx, a, r, policies...)
		trace = append(trace, scheduling.CandidatePolicyTrace{AccountID: a.ID, Allowed: d.Allowed, RejectedPolicy: d.RejectedPolicy})
		if !d.Allowed {
			continue
		}
		candidates = append(candidates, a)
		if r.Sticky {
			validSticky = true
		}
	}
	if !validSticky && stickyID > 0 && sessionHash != "" && s.cache != nil {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		stickyID = 0
	}
	if len(candidates) == 0 {
		stats.CandidateCount = 0
		stats.EmptyPool = true
		slog.Warn("gateway_scheduler_no_candidates_reject_trace", "platform", platform, "group_id", derefGroupID(groupID), "model", requestedModel, "reject_trace", trace)
		return nil, stats, ErrNoAvailableAccounts
	}
	stats.CandidateCount = len(candidates)
	loads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, a := range candidates {
		loads = append(loads, AccountWithConcurrency{ID: a.ID, MaxConcurrency: a.Concurrency})
	}
	var loadMap map[int64]*AccountLoadInfo
	if s.concurrencyService != nil {
		loadMap, _ = s.concurrencyService.GetAccountsLoadBatch(ctx, loads)
	}
	s.reportGatewayCandidateSnapshots(ctx, candidates, loadMap)
	order := simpleAccountOrder(candidates, loadMap, s.ttftLatencyPreference(requestedModel))
	// Explicit model routing remains a preferred pool, not a second scoring engine.
	var routeIDs []int64
	if group != nil && group.Platform == PlatformAnthropic && requestedModel != "" {
		routeIDs = group.GetRoutingAccountIDs(requestedModel)
	}
	if len(routeIDs) > 0 {
		routed, others := make([]*Account, 0, len(order)), make([]*Account, 0, len(order))
		for _, a := range order {
			if containsInt64(routeIDs, a.ID) {
				routed = append(routed, a)
			} else {
				others = append(others, a)
			}
		}
		order = append(routed, others...)
	}
	if stickyID > 0 && (len(routeIDs) == 0 || containsInt64(routeIDs, stickyID)) {
		order = preferSimpleAccount(order, stickyID)
	}
	recheck := func(a *Account) *Account {
		if s.accountRepo != nil {
			fresh, e := s.accountRepo.GetByID(ctx, a.ID)
			if e != nil || fresh == nil {
				return nil
			}
			a = fresh
		} else if s.schedulerSnapshot != nil {
			fresh, e := s.schedulerSnapshot.GetAccount(ctx, a.ID)
			if e != nil || fresh == nil {
				return nil
			}
			a = fresh
		}
		if !scheduling.GroupMembershipPolicy.Allows(ctx, a, scheduling.Request{GroupID: groupID, IncludeFlatGroupIDs: true}) || !allowed(a) {
			return nil
		}
		return a
	}
	for _, candidate := range order {
		a := recheck(candidate)
		if a == nil {
			continue
		}
		result, acquireErr := s.tryAcquireAccountSlot(ctx, requestedModel, a.ID, a.Concurrency)
		if acquireErr != nil {
			return nil, stats, acquireErr
		}
		if result != nil && result.ModelLimited {
			return &AccountSelectionResult{Account: a, ModelLimited: true}, stats, nil
		}
		if result == nil || !result.Acquired {
			continue
		}
		if sessionHash != "" && s.cache != nil {
			_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, a.ID, stickySessionTTL)
		}
		slog.Info("account_simple_selection", "platform", platform, "group_id", derefGroupID(groupID), "model", requestedModel, "selected_account_id", a.ID, "candidate_count", len(candidates))
		return &AccountSelectionResult{Account: a, Acquired: true, ReleaseFunc: result.ReleaseFunc}, stats, nil
	}
	for _, candidate := range order {
		a := recheck(candidate)
		if a == nil {
			continue
		}
		return &AccountSelectionResult{Account: a, WaitPlan: newFallbackWaitPlan(a.ID, a.Concurrency, cfg)}, stats, nil
	}
	// 候选存在但拿不到槽位/无等待计划：属于"忙池"，调用方不做 DB 复核。
	return nil, stats, ErrNoAvailableAccounts
}

func (s *GatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	return schedulingConfigFrom(s.cfg)
}

func (s *GatewayService) withGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
	}
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && existing != nil && existing.ID == group.ID && IsGroupContextValid(existing) {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, group)
}

func (s *GatewayService) groupFromContext(ctx context.Context, groupID int64) *Group {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group
	}
	return nil
}

func (s *GatewayService) resolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group := s.groupFromContext(ctx, groupID); group != nil {
		return group, nil
	}
	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
	}
	return group, nil
}

func (s *GatewayService) ResolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	return s.resolveGroupByID(ctx, groupID)
}

func (s *GatewayService) resolveGatewayGroup(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, nil, nil
	}

	currentID := *groupID
	visited := map[int64]struct{}{}
	for {
		if _, seen := visited[currentID]; seen {
			return nil, nil, fmt.Errorf("fallback group cycle detected")
		}
		visited[currentID] = struct{}{}

		group, err := s.resolveGroupByID(ctx, currentID)
		if err != nil {
			return nil, nil, err
		}

		if !group.ClaudeCodeOnly || IsClaudeCodeClient(ctx) {
			return group, &currentID, nil
		}

		if group.FallbackGroupID == nil {
			return nil, nil, ErrClaudeCodeOnly
		}
		currentID = *group.FallbackGroupID
	}
}

// checkClaudeCodeRestriction 检查分组的 Claude Code 客户端限制
// 如果分组启用了 claude_code_only 且请求不是来自 Claude Code 客户端：
//   - 有降级分组：返回降级分组的 ID
//   - 无降级分组：返回 ErrClaudeCodeOnly 错误
func (s *GatewayService) checkClaudeCodeRestriction(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, groupID, nil
	}

	// 强制平台模式不检查 Claude Code 限制
	if forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string); hasForcePlatform && forcePlatform != "" {
		return nil, groupID, nil
	}

	group, resolvedID, err := s.resolveGatewayGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}

	return group, resolvedID, nil
}

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64, group *Group) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
	}
	if group != nil {
		return group.Platform, false, nil
	}
	if platform, ok := ctx.Value(gatewayPlatformHintContextKey{}).(string); ok && platform != "" {
		return platform, false, nil
	}
	if groupID != nil {
		group, err := s.resolveGroupByID(ctx, *groupID)
		if err != nil {
			return "", false, err
		}
		return group.Platform, false, nil
	}
	return PlatformAnthropic, false, nil
}

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	repoPlatforms := []string{platform}
	var repoFilter func(Account) bool
	if useMixed {
		repoPlatforms = []string{platform, PlatformAntigravity}
		repoFilter = func(account Account) bool {
			return account.Platform != PlatformAntigravity || account.IsMixedSchedulingEnabled()
		}
	}
	var snapshot scheduling.AccountListSnapshot
	if s.schedulerSnapshot != nil {
		snapshot = s.schedulerSnapshot
	}
	result, err := scheduling.ListAccounts(ctx, snapshot, s.accountRepo, scheduling.AccountListRequest{
		GroupID:          groupID,
		Platform:         platform,
		HasForcePlatform: hasForcePlatform,
		UseMixed:         useMixed,
		RepoPlatforms:    repoPlatforms,
		RepoFilter:       repoFilter,
	})
	if err != nil {
		if !result.FromSnapshot {
			slog.Debug("account_scheduling_list_failed",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"error", err)
		}
		return result.Accounts, result.UseMixed, err
	}
	accounts := result.Accounts
	if result.FromSnapshot {
		slog.Debug("account_scheduling_list_snapshot",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"use_mixed", result.UseMixed,
			"count", len(accounts))
	} else if result.UseMixed {
		slog.Debug("account_scheduling_list_mixed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"raw_count", result.RawCount,
			"filtered_count", len(accounts))
	} else {
		slog.Debug("account_scheduling_list_single",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"count", len(accounts))
	}
	logSchedulingAccountDetails(ctx, accounts)
	return accounts, result.UseMixed, nil
}

func logSchedulingAccountDetails(ctx context.Context, accounts []Account) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}
	for _, account := range accounts {
		slog.Debug("account_scheduling_account_detail",
			"account_id", account.ID,
			"name", account.Name,
			"platform", account.Platform,
			"type", account.Type,
			"status", account.Status,
			"tls_fingerprint", account.IsTLSFingerprintEnabled())
	}
}

// IsSingleAntigravityAccountGroup 检查指定分组是否只有一个 antigravity 平台的可调度账号。
// 用于 Handler 层在首次请求时提前设置 SingleAccountRetry context，
// 避免单账号分组收到 503 时错误地设置模型限流标记导致后续请求连续快速失败。
func (s *GatewayService) IsSingleAntigravityAccountGroup(ctx context.Context, groupID *int64) bool {
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAntigravity, true)
	if err != nil {
		return false
	}
	return len(accounts) == 1
}

func (s *GatewayService) tryAcquireAccountSlot(ctx context.Context, requestedModel string, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	return tryAcquireAccountSlot(ctx, s.concurrencyService, requestedModel, accountID, maxConcurrency)
}

// isAccountSchedulableForQuota 检查账号是否在配额限制内
// 适用于配置了 quota_limit 的 apikey 类型账号
func (s *GatewayService) accountPassesQuota(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return true
	}
	return !account.IsQuotaExceeded()
}

func (s *GatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	return newHydratedSelectionResult(ctx, s.schedulerSnapshot, account, "gateway", acquired, release, waitPlan)
}

// newAcquiredSelectionResult 构造已抢到并发槽的选择结果；若 hydrate 失败则立即释放槽位，
// 避免槽位泄漏直到 Redis TTL 过期（与 OpenAIGatewayService.newAcquiredSelectionResult 对称）。
func (s *GatewayService) newAcquiredSelectionResult(ctx context.Context, account *Account, release func()) (*AccountSelectionResult, error) {
	return newHydratedAcquiredResult(ctx, s.schedulerSnapshot, account, "gateway", release)
}

// newModelLimitedSelectionResult 构造模型级预算已满的拒绝结果。
// WaitPlan 恒为 nil，Handler 层据此直接 429 让路，不进入账号等待队列。
func (s *GatewayService) newModelLimitedSelectionResult(ctx context.Context, account *Account) (*AccountSelectionResult, error) {
	return newHydratedModelLimitedResult(ctx, s.schedulerSnapshot, account, "gateway")
}

type selectionFailureStats struct {
	Total              int
	Eligible           int
	Excluded           int
	Unschedulable      int
	PlatformFiltered   int
	ModelUnsupported   int
	ModelRateLimited   int
	SamplePlatformIDs  []int64
	SampleMappingIDs   []int64
	SampleRateLimitIDs []string
}

type selectionFailureDiagnosis struct {
	Category string
	Detail   string
}

func (s *GatewayService) collectSelectionFailureStats(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := selectionFailureStats{
		Total: len(accounts),
	}

	for i := range accounts {
		acc := &accounts[i]
		diagnosis := s.diagnoseSelectionFailure(ctx, acc, requestedModel, platform, excludedIDs, allowMixedScheduling)
		switch diagnosis.Category {
		case "excluded":
			stats.Excluded++
		case "unschedulable":
			stats.Unschedulable++
		case "platform_filtered":
			stats.PlatformFiltered++
			stats.SamplePlatformIDs = appendSelectionFailureSampleID(stats.SamplePlatformIDs, acc.ID)
		case "model_unsupported":
			stats.ModelUnsupported++
			stats.SampleMappingIDs = appendSelectionFailureSampleID(stats.SampleMappingIDs, acc.ID)
		case "model_rate_limited":
			stats.ModelRateLimited++
			remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
			stats.SampleRateLimitIDs = appendSelectionFailureRateSample(stats.SampleRateLimitIDs, acc.ID, remaining)
		default:
			stats.Eligible++
		}
	}

	return stats
}

func (s *GatewayService) diagnoseSelectionFailure(
	ctx context.Context,
	acc *Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureDiagnosis {
	if acc == nil {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "account_nil"}
	}
	decision := scheduling.EvaluateCandidate(ctx, acc, scheduling.Request{
		Platform:           platform,
		RequestedModel:     requestedModel,
		ExcludedIDs:        excludedIDs,
		AllowMixedPlatform: allowMixedScheduling,
	}, s.gatewayCandidatePolicies()...)
	switch decision.RejectedPolicy {
	case "not_excluded":
		return selectionFailureDiagnosis{Category: "excluded"}
	case "schedulable":
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "generic_unschedulable"}
	case "gateway_platform":
		return selectionFailureDiagnosis{
			Category: "platform_filtered",
			Detail:   fmt.Sprintf("account_platform=%s requested_platform=%s", acc.Platform, strings.TrimSpace(platform)),
		}
	case "model_capability":
		return selectionFailureDiagnosis{
			Category: "model_unsupported",
			Detail:   fmt.Sprintf("model=%s", requestedModel),
		}
	case "schedulable_for_model":
		remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
		return selectionFailureDiagnosis{
			Category: "model_rate_limited",
			Detail:   fmt.Sprintf("remaining=%s", remaining),
		}
	}
	return selectionFailureDiagnosis{Category: "eligible"}
}

func appendSelectionFailureSampleID(samples []int64, id int64) []int64 {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, id)
}

func appendSelectionFailureRateSample(samples []string, accountID int64, remaining time.Duration) []string {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, fmt.Sprintf("%d(%s)", accountID, remaining))
}

// isModelSupportedByAccountWithContext 根据账户平台检查模型支持（带 context）
// Antigravity is Gemini-only; thinking is carried in generationConfig and does
// not select a separate suffixed model.
func (s *GatewayService) isModelSupportedByAccountWithContext(ctx context.Context, account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		// 使用与转发阶段一致的映射逻辑：自定义映射优先 → 默认映射兜底
		mapped := mapAntigravityModel(account, requestedModel)
		if mapped == "" {
			return false
		}
		return true
	}
	return s.isModelSupportedByAccount(account, requestedModel)
}

// isModelSupportedByAccount 根据账户平台检查模型支持（无 context，用于非 Antigravity 平台）
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	// OpenAI 透传模式：仅替换认证，允许所有模型
	if account.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
		return true
	}
	// OAuth/SetupToken 账号使用 Anthropic 标准映射（短ID → 长ID）
	if account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		requestedModel = claude.NormalizeModelID(requestedModel)
	}
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
}
