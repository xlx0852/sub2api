package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

const (
	openAIAccountScheduleLayerPreviousResponse = "previous_response_id"
	openAIAccountScheduleLayerUserPin          = "user_pin"
	openAIAccountScheduleLayerSessionSticky    = "session_hash"
	openAIAccountScheduleLayerLoadBalance      = "load_balance"
	openAIAdvancedSchedulerSettingKey          = "openai_advanced_scheduler_enabled"
)

const (
	openAIAdvancedSchedulerSettingCacheTTL  = 5 * time.Second
	openAIAdvancedSchedulerSettingDBTimeout = 2 * time.Second
)

const (
	openAIQuotaHeadroomNeutralFactor      = 0.5
	openAIQuotaHeadroomSecondaryLowRemain = 0.10
	openAIQuotaHeadroomSnapshotStaleAfter = 8 * time.Hour
	grokQuotaHeadroomSnapshotStaleAfter   = 24 * time.Hour
)

const (
	openAICompactHardPenaltyMinDuration = 30 * time.Second
	openAICompactHardPenaltyDuration    = 10 * time.Minute
)

type OpenAIAccountScheduleRequest struct {
	simpleGroup             *Group
	GroupID                 *int64
	Platform                string
	SessionHash             string
	StickyAccountID         int64
	PreviousResponseID      string
	PreviousResponseCanMove bool
	RequestedModel          string
	RequiredTransport       OpenAIUpstreamTransport
	RequiredCapability      OpenAIEndpointCapability
	RequiredImageCapability OpenAIImagesCapability
	RequireCompact          bool
	RequiredCredentialClass OpenAICredentialClass
	ExcludedIDs             map[int64]struct{}
	// APIKeyID 本地 API Key ID（用户级钉号用）。0 表示未鉴权路径（如管理端探测），跳过钉号。
	APIKeyID int64
	// UserPinAccountID 预解析的钉号账号 ID；0 表示未钉或功能关闭。
	UserPinAccountID int64
}

type OpenAIAccountScheduleDecision struct {
	Layer               string
	StickyPreviousHit   bool
	StickySessionHit    bool
	UserPinHit          bool
	UserPinBroken       string
	StickyReevalSwitch  bool
	CandidateCount      int
	TopK                int
	LatencyMs           int64
	LoadSkew            float64
	SelectedAccountID   int64
	SelectedAccountType string
}

type OpenAIAccountSchedulerMetricsSnapshot struct {
	SelectTotal              int64
	StickyPreviousHitTotal   int64
	StickySessionHitTotal    int64
	LoadBalanceSelectTotal   int64
	AccountSwitchTotal       int64
	SchedulerLatencyMsTotal  int64
	SchedulerLatencyMsAvg    float64
	StickyHitRatio           float64
	AccountSwitchRate        float64
	LoadSkewAvg              float64
	RuntimeStatsAccountCount int
}

type OpenAIAccountScheduler interface {
	Select(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	ReportResult(accountID int64, success bool, firstTokenMs *int)
	ReportCompactResult(accountID int64, success bool, duration time.Duration)
	ReportSwitch()
	SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot
}

type openAIAccountSchedulerMetrics struct {
	selectTotal            atomic.Int64
	stickyPreviousHitTotal atomic.Int64
	stickySessionHitTotal  atomic.Int64
	loadBalanceSelectTotal atomic.Int64
	accountSwitchTotal     atomic.Int64
	latencyMsTotal         atomic.Int64
	loadSkewMilliTotal     atomic.Int64
}

type openAIAccountLoadPlan struct {
	allCandidates  []openAIAccountCandidate
	candidates     []openAIAccountCandidate
	selectionOrder []openAIAccountCandidate
	candidateCount int
	topK           int // compatibility metric: all eligible candidates, not a truncation limit
	loadSkew       float64
}

type openAIAccountLoadSelectionAttempt struct {
	result              *AccountSelectionResult
	selectionOrder      []openAIAccountCandidate
	candidateCount      int
	topK                int
	loadSkew            float64
	compactBlocked      bool
	noCompactCandidates bool
	err                 error
}

func (m *openAIAccountSchedulerMetrics) recordSelect(decision OpenAIAccountScheduleDecision) {
	if m == nil {
		return
	}
	m.selectTotal.Add(1)
	m.latencyMsTotal.Add(decision.LatencyMs)
	m.loadSkewMilliTotal.Add(int64(math.Round(decision.LoadSkew * 1000)))
	if decision.StickyPreviousHit {
		m.stickyPreviousHitTotal.Add(1)
	}
	if decision.StickySessionHit {
		m.stickySessionHitTotal.Add(1)
	}
	if decision.Layer == openAIAccountScheduleLayerLoadBalance {
		m.loadBalanceSelectTotal.Add(1)
	}
}

func (m *openAIAccountSchedulerMetrics) recordSwitch() {
	if m == nil {
		return
	}
	m.accountSwitchTotal.Add(1)
}

type openAIAccountRuntimeStats struct {
	accounts     sync.Map
	accountCount atomic.Int64
}

type openAIAccountRuntimeStat struct {
	errorRateEWMABits        atomic.Uint64
	ttftEWMABits             atomic.Uint64
	compactErrorRateEWMABits atomic.Uint64
	compactLatencyEWMABits   atomic.Uint64
	compactPenaltyUntilNano  atomic.Int64
}

func newOpenAIAccountRuntimeStats() *openAIAccountRuntimeStats {
	return &openAIAccountRuntimeStats{}
}

func (s *openAIAccountRuntimeStats) loadOrCreate(accountID int64) *openAIAccountRuntimeStat {
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*openAIAccountRuntimeStat)
		if stat != nil {
			return stat
		}
	}

	stat := &openAIAccountRuntimeStat{}
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	stat.compactLatencyEWMABits.Store(math.Float64bits(math.NaN()))
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		return stat
	}
	existing, _ := actual.(*openAIAccountRuntimeStat)
	if existing != nil {
		return existing
	}
	return stat
}

func updateEWMAAtomic(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := alpha*sample + (1-alpha)*oldValue
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
		}
	}
}

func updateEWMAAtomicAllowNaN(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := sample
		if !math.IsNaN(oldValue) {
			newValue = alpha*sample + (1-alpha)*oldValue
		}
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
		}
	}
}

func (s *openAIAccountRuntimeStats) report(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || accountID <= 0 {
		return
	}
	const alpha = 0.2
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
	}
	updateEWMAAtomic(&stat.errorRateEWMABits, errorSample, alpha)

	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttft := float64(*firstTokenMs)
		ttftBits := math.Float64bits(ttft)
		for {
			oldBits := stat.ttftEWMABits.Load()
			oldValue := math.Float64frombits(oldBits)
			if math.IsNaN(oldValue) {
				if stat.ttftEWMABits.CompareAndSwap(oldBits, ttftBits) {
					break
				}
				continue
			}
			newValue := alpha*ttft + (1-alpha)*oldValue
			if stat.ttftEWMABits.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
				break
			}
		}
	}
}

func (s *openAIAccountRuntimeStats) snapshot(accountID int64) (errorRate float64, ttft float64, hasTTFT bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false
	}
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
	}
	return errorRate, ttftValue, true
}

func (s *openAIAccountRuntimeStats) has(accountID int64) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	_, ok := s.accounts.Load(accountID)
	return ok
}

func (s *openAIAccountRuntimeStats) reportCompact(accountID int64, success bool, duration time.Duration) {
	if s == nil || accountID <= 0 {
		return
	}
	const alpha = 0.25
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
	}
	updateEWMAAtomic(&stat.compactErrorRateEWMABits, errorSample, alpha)

	if duration > 0 {
		updateEWMAAtomicAllowNaN(&stat.compactLatencyEWMABits, float64(duration.Milliseconds()), alpha)
	}
	if success {
		stat.compactPenaltyUntilNano.Store(0)
		return
	}
	if duration >= openAICompactHardPenaltyMinDuration {
		stat.compactPenaltyUntilNano.Store(time.Now().Add(openAICompactHardPenaltyDuration).UnixNano())
	}
}

func (s *openAIAccountRuntimeStats) snapshotCompact(accountID int64) (errorRate float64, latencyMs float64, hasLatency bool, penaltyActive bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false, false
	}
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false, false
	}
	penaltyActive = stat.compactPenaltyUntilNano.Load() > time.Now().UnixNano()
	errorRate = clamp01(math.Float64frombits(stat.compactErrorRateEWMABits.Load()))
	latencyValue := math.Float64frombits(stat.compactLatencyEWMABits.Load())
	if math.IsNaN(latencyValue) {
		return errorRate, 0, false, penaltyActive
	}
	return errorRate, latencyValue, true, penaltyActive
}

func (s *openAIAccountRuntimeStats) size() int {
	if s == nil {
		return 0
	}
	return int(s.accountCount.Load())
}

type defaultOpenAIAccountScheduler struct {
	service *OpenAIGatewayService
	metrics openAIAccountSchedulerMetrics
	stats   *openAIAccountRuntimeStats
}

func newDefaultOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
	}
	return &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
	}
}

func (s *defaultOpenAIAccountScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
	}()

	if class := openAICredentialFailoverClass(ctx); class != OpenAICredentialClassUnspecified {
		req.RequiredCredentialClass = class
	}
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		group, err := s.service.schedulerSnapshot.GetGroupByID(ctx, *req.GroupID)
		if err != nil {
			return nil, decision, err
		}
		if group == nil {
			return nil, decision, noAvailableOpenAISelectionErrorWithReason(req.RequestedModel, "group_unavailable", false)
		}
		req.simpleGroup = group
	}
	originalPin := req.UserPinAccountID
	s.prepareSimplePreferences(ctx, &req)
	if originalPin > 0 && req.UserPinAccountID == 0 {
		decision.UserPinBroken = "request_incompatible"
		if scheduling.IsExcluded(req.ExcludedIDs, originalPin) {
			decision.UserPinBroken = "excluded_by_failover"
		}
	}
	defer func() {
		slog.Info("account_simple_selection", "platform", normalizeOpenAICompatiblePlatform(req.Platform), "group_id", derefGroupID(req.GroupID),
			"model", req.RequestedModel, "selected_account_id", decision.SelectedAccountID,
			"layer", decision.Layer, "candidate_count", decision.CandidateCount,
			"required_credential_class", string(req.RequiredCredentialClass))
	}()
	if normalizeOpenAICompatiblePlatform(req.Platform) == PlatformOpenAI && strings.TrimSpace(req.PreviousResponseID) != "" {
		selection, err := s.selectSimplePreviousResponse(ctx, req)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil {
			decision.Layer = openAIAccountScheduleLayerPreviousResponse
			decision.StickyPreviousHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			return selection, decision, nil
		}
	}

	selection, candidateCount, topK, loadSkew, err := s.selectByLoadBalance(ctx, req)
	if err != nil && errors.Is(err, ErrNoAvailableAccounts) && len(req.ExcludedIDs) == 0 && s.service != nil && s.service.schedulerSnapshot != nil {
		refreshed, _, refreshErr := s.service.schedulerSnapshot.RefreshSchedulableAccounts(ctx, req.GroupID, normalizeOpenAICompatiblePlatform(req.Platform), false)
		if refreshErr == nil {
			slog.Warn("openai_scheduler_empty_pool_db_recheck",
				"group_id", derefGroupID(req.GroupID),
				"platform", normalizeOpenAICompatiblePlatform(req.Platform),
				"model", req.RequestedModel,
				"snapshot_candidate_count", candidateCount,
				"db_candidate_count", len(refreshed),
			)
			selection, candidateCount, topK, loadSkew, err = s.selectByLoadBalance(ctx, req)
			if err != nil {
				slog.Warn("openai_scheduler_empty_pool_db_recheck_unrecovered",
					"group_id", derefGroupID(req.GroupID),
					"platform", normalizeOpenAICompatiblePlatform(req.Platform),
					"model", req.RequestedModel,
					"db_candidate_count", len(refreshed),
					"error", err,
				)
			}
		} else if !errors.Is(refreshErr, ErrSchedulerFallbackLimited) {
			slog.Warn("openai_scheduler_empty_pool_db_recheck_failed",
				"group_id", derefGroupID(req.GroupID),
				"platform", normalizeOpenAICompatiblePlatform(req.Platform),
				"model", req.RequestedModel,
				"error", refreshErr,
			)
		}
	}
	decision.Layer = openAIAccountScheduleLayerLoadBalance
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		decision.UserPinHit = req.UserPinAccountID > 0 && selection.Account.ID == req.UserPinAccountID
		if decision.UserPinHit {
			s.service.refreshUserPinTTL(ctx, req.GroupID, req.APIKeyID)
		}
		decision.StickySessionHit = req.StickyAccountID > 0 && selection.Account.ID == req.StickyAccountID
		if decision.UserPinHit {
			decision.Layer = openAIAccountScheduleLayerUserPin
		} else if decision.StickySessionHit {
			decision.Layer = openAIAccountScheduleLayerSessionSticky
		}
	}
	return selection, decision, nil
}

func openAIStickyAccountMatchesGroup(account *Account, groupID *int64) bool {
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{GroupID: groupID, IncludeFlatGroupIDs: true},
		scheduling.GroupMembershipPolicy,
	).Allowed
}

type openAIAccountCandidate struct {
	account  *Account
	loadInfo *AccountLoadInfo
	priority int
}

func openAIAccountCandidateIDs(candidates []openAIAccountCandidate) []int64 {
	result := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.account == nil {
			continue
		}
		result = append(result, candidate.account.ID)
	}
	return result
}

func openAIAccountPointerIDs(accounts []*Account) []int64 {
	result := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			result = append(result, account.ID)
		}
	}
	return result
}

func (s *defaultOpenAIAccountScheduler) buildOpenAIAccountLoadPlan(ctx context.Context, req OpenAIAccountScheduleRequest, filtered []*Account, loadMap map[int64]*AccountLoadInfo) openAIAccountLoadPlan {
	return s.buildSimpleLoadPlan(req, filtered, loadMap, uint64(time.Now().UnixNano()))
}

func (s *defaultOpenAIAccountScheduler) tryAcquireOpenAISelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	selectionOrder []openAIAccountCandidate,
) (*AccountSelectionResult, bool, error) {
	compactBlocked := false
	for i := 0; i < len(selectionOrder); i++ {
		candidate := selectionOrder[i]
		fresh := s.recheckLoadCandidate(ctx, candidate.account, req)
		if fresh == nil {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			continue
		}
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, req.RequestedModel, fresh.ID, fresh.Concurrency)
		if acquireErr != nil {
			return nil, compactBlocked, acquireErr
		}
		if result != nil && result.Acquired {
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, fresh.ID)
			}
			return &AccountSelectionResult{
				Account:     fresh,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
			}, compactBlocked, nil
		}
		if result != nil && result.ModelLimited {
			// 模型级并发预算已满：立即让路，不进入账号等待队列。
			return &AccountSelectionResult{
				Account:      fresh,
				ModelLimited: true,
			}, compactBlocked, nil
		}
	}
	return nil, compactBlocked, nil
}

func (s *defaultOpenAIAccountScheduler) selectByLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int, int, float64, error) {
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if len(accounts) == 0 {
		return nil, 0, 0, 0, noAvailableOpenAISelectionErrorWithReason(req.RequestedModel, "platform_empty", false)
	}

	filtered := make([]*Account, 0, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	shadowPolicyTrace := make([]scheduling.CandidatePolicyTrace, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		d := s.simpleCandidateDecision(ctx, account, req)
		shadowPolicyTrace = append(shadowPolicyTrace, scheduling.CandidatePolicyTrace{AccountID: account.ID, Allowed: d.Allowed, RejectedPolicy: d.RejectedPolicy})
		if d.Allowed {
			filtered = append(filtered, account)
			loadReq = append(loadReq, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.Concurrency})
		}
	}
	if len(filtered) == 0 {
		// 空候选时输出每个平台级候选被哪条 policy 拒绝的明细，区分
		// "平台级无候选"与"模型级/能力级无候选"，避免 no-available 误导读。
		slog.Warn("openai_scheduler_no_candidates_reject_trace",
			"platform", normalizeOpenAICompatiblePlatform(req.Platform),
			"group_id", derefGroupID(req.GroupID),
			"model", req.RequestedModel,
			"require_compact", req.RequireCompact,
			"required_capability", string(req.RequiredCapability),
			"required_transport", string(req.RequiredTransport),
			"raw_candidate_count", len(accounts),
			"reject_trace", shadowPolicyTrace,
		)
		return nil, 0, 0, 0, noAvailableOpenAISelectionErrorWithReason(req.RequestedModel, "filtered_all", false)
	}

	// 请求级预取:对全部候选(含 shadow 母账号)做一次性 GetByIDs,供 recheck / parent
	// 解析查 map,替代 tryAcquireOpenAISelectionOrder / finishLoadBalanceSelectionFallback
	// 循环内逐候选裸 GetByID(N+1)。失败或快照不可用时跳过,各调用点回退逐账号查询。
	ctx = s.service.prefetchOpenAIAccountsForRequest(ctx, filtered)

	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}

	attempt := s.trySelectByLoadBalancePool(ctx, req, filtered, loadMap)
	if attempt.err != nil {
		return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
	}
	if attempt.result != nil {
		return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
	}
	return s.finishLoadBalanceSelectionFallback(ctx, req, attempt)
}

func (s *defaultOpenAIAccountScheduler) trySelectByLoadBalancePool(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	filtered []*Account,
	loadMap map[int64]*AccountLoadInfo,
) openAIAccountLoadSelectionAttempt {
	plan := s.buildOpenAIAccountLoadPlan(ctx, req, filtered, loadMap)
	attempt := openAIAccountLoadSelectionAttempt{
		selectionOrder: plan.selectionOrder,
		candidateCount: plan.candidateCount,
		topK:           plan.topK,
		loadSkew:       plan.loadSkew,
	}

	if len(attempt.selectionOrder) == 0 {
		attempt.compactBlocked = req.RequireCompact && len(plan.allCandidates) > 0
		return attempt
	}

	result, compactBlocked, acquireErr := s.tryAcquireOpenAISelectionOrder(ctx, req, attempt.selectionOrder)
	attempt.compactBlocked = compactBlocked
	if acquireErr != nil {
		attempt.err = acquireErr
		return attempt
	}
	if result != nil {
		attempt.result = result
		return attempt
	}

	if s.service.concurrencyService != nil {
		loadReq := buildOpenAIAccountLoadRequest(filtered)
		if freshLoadMap, loadErr := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, loadReq); loadErr == nil {
			freshPlan := s.buildOpenAIAccountLoadPlan(ctx, req, filtered, freshLoadMap)
			if len(freshPlan.selectionOrder) > 0 {
				freshResult, freshCompactBlocked, freshAcquireErr := s.tryAcquireOpenAISelectionOrder(ctx, req, freshPlan.selectionOrder)
				if freshAcquireErr != nil {
					attempt.err = freshAcquireErr
					return attempt
				}
				if freshResult != nil {
					attempt.result = freshResult
					attempt.selectionOrder = freshPlan.selectionOrder
					attempt.candidateCount = freshPlan.candidateCount
					attempt.topK = freshPlan.topK
					attempt.loadSkew = freshPlan.loadSkew
					return attempt
				}
				attempt.compactBlocked = attempt.compactBlocked || freshCompactBlocked
				attempt.selectionOrder = freshPlan.selectionOrder
				attempt.candidateCount = freshPlan.candidateCount
				attempt.topK = freshPlan.topK
				attempt.loadSkew = freshPlan.loadSkew
			}
		}
	}

	return attempt
}

func buildOpenAIAccountLoadRequest(accounts []*Account) []AccountWithConcurrency {
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.Concurrency,
		})
	}
	return loadReq
}

func (s *defaultOpenAIAccountScheduler) finishLoadBalanceSelectionFallback(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	attempt openAIAccountLoadSelectionAttempt,
) (*AccountSelectionResult, int, int, float64, error) {
	candidateCount := attempt.candidateCount
	topK := attempt.topK
	loadSkew := attempt.loadSkew

	if len(attempt.selectionOrder) == 0 {
		return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionErrorWithReason(req.RequestedModel, "model_filtered", attempt.compactBlocked)
	}

	cfg := s.service.schedulingConfig()
	compactBlocked := attempt.compactBlocked
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	for _, candidate := range attempt.selectionOrder {
		fresh := s.recheckLoadCandidate(ctx, candidate.account, req)
		if fresh == nil {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			compactBlocked = true
			continue
		}
		return &AccountSelectionResult{
			Account:  fresh,
			WaitPlan: newFallbackWaitPlan(fresh.ID, fresh.Concurrency, cfg),
		}, candidateCount, topK, loadSkew, nil
	}

	return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionErrorWithReason(req.RequestedModel, "acquire_or_recheck_failed", compactBlocked)
}

func (s *defaultOpenAIAccountScheduler) isAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	var service *OpenAIGatewayService
	if s != nil {
		service = s.service
	}
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{RequiredTransport: string(requiredTransport)},
		openAITransportPolicy{service: service},
	).Allowed
}

func (s *defaultOpenAIAccountScheduler) lookupShadowParentAccount(ctx context.Context, id int64) *Account {
	if s == nil || s.service == nil {
		return nil
	}
	if s.service.schedulerSnapshot != nil {
		if account, err := s.service.schedulerSnapshot.GetAccount(ctx, id); err == nil && account != nil {
			return account
		}
	}
	if s.service.accountRepo == nil {
		return nil
	}
	account, _ := s.service.accountRepo.GetByID(ctx, id)
	return account
}

func (s *defaultOpenAIAccountScheduler) isAccountRequestCompatible(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) bool {
	return s.accountRequestPolicyDecision(ctx, account, req).Allowed
}

func (s *defaultOpenAIAccountScheduler) accountRequestPolicyDecision(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) scheduling.PolicyDecision {
	if s == nil || s.service == nil {
		return scheduling.PolicyDecision{RejectedPolicy: "scheduler_unavailable"}
	}
	return scheduling.EvaluateCandidate(
		ctx,
		account,
		openAICandidatePolicyRequest(req),
		openAIRuntimePolicy{service: s.service},
		openAIShadowParentPolicy{scheduler: s},
		openAIRequestedModelPolicy,
		openAIUpstreamChannelPolicy{service: s.service},
		openAIAllCapabilitiesPolicy,
	)
}

func (s *defaultOpenAIAccountScheduler) ReportResult(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || s.stats == nil {
		return
	}
	s.stats.report(accountID, success, firstTokenMs)
}

func (s *defaultOpenAIAccountScheduler) ReportCompactResult(accountID int64, success bool, duration time.Duration) {
	if s == nil || s.stats == nil {
		return
	}
	s.stats.reportCompact(accountID, success, duration)
}

func (s *defaultOpenAIAccountScheduler) ReportSwitch() {
	if s == nil {
		return
	}
	s.metrics.recordSwitch()
}

func (s *defaultOpenAIAccountScheduler) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	if s == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}

	selectTotal := s.metrics.selectTotal.Load()
	prevHit := s.metrics.stickyPreviousHitTotal.Load()
	sessionHit := s.metrics.stickySessionHitTotal.Load()
	switchTotal := s.metrics.accountSwitchTotal.Load()
	latencyTotal := s.metrics.latencyMsTotal.Load()
	loadSkewTotal := s.metrics.loadSkewMilliTotal.Load()

	snapshot := OpenAIAccountSchedulerMetricsSnapshot{
		SelectTotal:              selectTotal,
		StickyPreviousHitTotal:   prevHit,
		StickySessionHitTotal:    sessionHit,
		LoadBalanceSelectTotal:   s.metrics.loadBalanceSelectTotal.Load(),
		AccountSwitchTotal:       switchTotal,
		SchedulerLatencyMsTotal:  latencyTotal,
		RuntimeStatsAccountCount: s.stats.size(),
	}
	if selectTotal > 0 {
		snapshot.SchedulerLatencyMsAvg = float64(latencyTotal) / float64(selectTotal)
		snapshot.StickyHitRatio = float64(prevHit+sessionHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
	}
	return snapshot
}

type openAIAdvancedSchedulerWeightOverrideSpec struct {
	key  string
	name string
}

func (s *OpenAIGatewayService) getOpenAIAccountScheduler(ctx context.Context) OpenAIAccountScheduler {
	if s == nil {
		return nil
	}

	s.openaiSchedulerOnce.Do(func() {
		if s.openaiAccountStats == nil {
			s.openaiAccountStats = newOpenAIAccountRuntimeStats()
		}
		if s.openaiScheduler == nil {
			s.openaiScheduler = newDefaultOpenAIAccountScheduler(s, s.openaiAccountStats)
		}
	})
	return s.openaiScheduler
}

func resetOpenAIAdvancedSchedulerSettingCacheForTest() {
	// 缓存已挂在 SettingService 实例上，测试隔离由各自 NewSettingService 保证。
}

func (s *OpenAIGatewayService) SelectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requireCompact bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.selectAccountThroughEngine(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, "", "", requireCompact, PlatformOpenAI, false)
}

// SelectAccountWithSchedulerForCapability 按能力要求调度账号。
// previousResponseCanMove 表示首包 input 可自行重建工具续链，previous_response_id 允许跨账号迁移
// （粘性加权模式下改为加权偏好而非硬粘连）。
func (s *OpenAIGatewayService) SelectAccountWithSchedulerForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
	previousResponseCanMove bool,
	platformOverride ...string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	platform := PlatformOpenAI
	if len(platformOverride) > 0 {
		platform = platformOverride[0]
	}
	return s.selectAccountThroughEngine(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, "", requireCompact, platform, previousResponseCanMove)
}

func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImages(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	selection, decision, err := s.selectAccountThroughEngine(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportHTTPSSE, "", requiredCapability, false, PlatformOpenAI, false)
	if err == nil && selection != nil && selection.Account != nil {
		return selection, decision, nil
	}
	// 如果要求 native 能力（如指定了模型）但没有可用的 APIKey 账号，回退到 basic（OAuth 账号）
	if requiredCapability == OpenAIImagesCapabilityNative {
		return s.selectAccountThroughEngine(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportHTTPSSE, "", OpenAIImagesCapabilityBasic, false, PlatformOpenAI, false)
	}
	return selection, decision, err
}

func (s *OpenAIGatewayService) selectAccountWithSchedulerStrategy(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
	requireCompact bool,
	platform string,
	previousResponseCanMove bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	platform = normalizeOpenAICompatiblePlatform(platform)
	decision := OpenAIAccountScheduleDecision{}
	scheduler := s.getOpenAIAccountScheduler(ctx)
	if scheduler == nil {
		return nil, decision, ErrNoAvailableAccounts
	}

	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, decision, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil && accountID > 0 {
			stickyAccountID = accountID
		}
	}
	userPinAPIKeyID := openAIUserPinAPIKeyIDFromContext(ctx)
	userPinAccountID := int64(0)
	if platform == PlatformOpenAI {
		userPinAccountID = s.getUserPinnedAccountID(ctx, groupID, userPinAPIKeyID)
	}
	requiredCredentialClass := openAICredentialFailoverClass(ctx)

	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:                 groupID,
		Platform:                platform,
		SessionHash:             sessionHash,
		StickyAccountID:         stickyAccountID,
		PreviousResponseID:      previousResponseID,
		PreviousResponseCanMove: previousResponseCanMove,
		RequestedModel:          requestedModel,
		RequiredTransport:       requiredTransport,
		RequiredCapability:      requiredCapability,
		RequiredImageCapability: requiredImageCapability,
		RequireCompact:          requireCompact,
		RequiredCredentialClass: requiredCredentialClass,
		ExcludedIDs:             excludedIDs,
		APIKeyID:                userPinAPIKeyID,
		UserPinAccountID:        userPinAccountID,
	})
	// 钉号绑定收口：最终选中的账号（钉号命中或负载均衡新选）都重钉/续钉。
	// 只钉 OAuth 号——API Key 池账号无共享指纹问题，不值得为之牺牲负载均衡。
	if err == nil && selection != nil && selection.Account != nil &&
		!decision.UserPinHit && userPinAPIKeyID > 0 && selection.Account.IsOpenAIOAuth() {
		s.bindUserPinAccount(ctx, groupID, userPinAPIKeyID, selection.Account.ID)
	}
	return selection, decision, err
}

func accountSupportsOpenAICapabilities(account *Account, requiredCapability OpenAIEndpointCapability, requiredImageCapability OpenAIImagesCapability) bool {
	if account == nil {
		return false
	}
	return account.SupportsOpenAIEndpointCapability(requiredCapability) &&
		account.SupportsOpenAIImageCapability(requiredImageCapability)
}

func (s *OpenAIGatewayService) isOpenAIAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || account == nil {
		return false
	}
	if requiredTransport == OpenAIUpstreamTransportResponsesWebsocketV2Ingress {
		if s.cfg == nil || !s.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled {
			return s.getOpenAIWSProtocolResolver().Resolve(account).Transport == OpenAIUpstreamTransportResponsesWebsocketV2
		}
		mode := account.ResolveOpenAIResponsesWebSocketV2Mode(s.cfg.Gateway.OpenAIWS.IngressModeDefault)
		switch mode {
		case OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough, OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModeShared, OpenAIWSIngressModeDedicated:
			return true
		default:
			return false
		}
	}
	return s.getOpenAIWSProtocolResolver().Resolve(account).Transport == requiredTransport
}

func (s *OpenAIGatewayService) ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	if s != nil && success {
		s.codexTurnStateFailureMu.Lock()
		s.codexTurnStateUpstream5xx.Delete(accountID)
		s.codexTurnStateFailureMu.Unlock()
	}

	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportResult(accountID, success, firstTokenMs)
}

func (s *OpenAIGatewayService) ReportOpenAICompactScheduleResult(accountID int64, success bool, duration time.Duration) {
	if s != nil && success {
		s.codexTurnStateFailureMu.Lock()
		s.codexTurnStateUpstream5xx.Delete(accountID)
		s.codexTurnStateFailureMu.Unlock()
	}

	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportCompactResult(accountID, success, duration)
}

func (s *OpenAIGatewayService) RecordOpenAIAccountSwitch() {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportSwitch()
}

func (s *OpenAIGatewayService) SnapshotOpenAIAccountSchedulerMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}
	return scheduler.SnapshotMetrics()
}

func (s *OpenAIGatewayService) openAIWSSessionStickyTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return openaiStickySessionTTL
}

type OpenAIAccountSchedulerScoreSnapshot struct {
	BaseScore             float64
	StickyScore           float64
	StickyScoreInfinity   bool
	StickyWeightedEnabled bool
}

// Legacy admin clients may still ask for composite scores. They are no longer
// calculated: an absent per-account snapshot prevents showing fictitious scores.
func (s *RateLimitService) BuildOpenAIAccountSchedulerScoreSnapshot(ctx context.Context, accounts []*Account, loadMap map[int64]*AccountLoadInfo) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	return nil
}
func BuildOpenAIAccountSchedulerScoreSnapshot(accounts []*Account, loadMap map[int64]*AccountLoadInfo) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	return nil
}

func openAIQuotaHeadroomSnapshotStale(extra map[string]any, now time.Time) bool {
	updatedRaw, ok := extra["codex_usage_updated_at"]
	if !ok {
		return true
	}
	updatedAt, err := parseTime(fmt.Sprint(updatedRaw))
	if err != nil {
		return true
	}
	return now.Sub(updatedAt) >= openAIQuotaHeadroomSnapshotStaleAfter
}

func openAIQuotaWindowResetAny(extra map[string]any, now time.Time, windows ...string) bool {
	for _, window := range windows {
		if openAIQuotaWindowReset(extra, window, now) {
			return true
		}
	}
	return false
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func calcLoadSkewByMoments(sum float64, sumSquares float64, count int) float64 {
	if count <= 1 {
		return 0
	}
	mean := sum / float64(count)
	variance := sumSquares/float64(count) - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}
