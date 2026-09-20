package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
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

type cachedOpenAIAdvancedSchedulerSetting struct {
	enabled                     bool
	stickyWeightedEnabled       bool
	subscriptionPriorityEnabled bool
	lbTopKOverride              int
	weightOverrides             map[string]float64
	expiresAt                   int64
}

type openAIAdvancedSchedulerRuntimeSettings struct {
	enabled                     bool
	stickyWeightedEnabled       bool
	subscriptionPriorityEnabled bool
	lbTopKOverride              int
	weightOverrides             map[string]float64
}

var stickyReevalLastAt sync.Map // sessionHash -> int64 unix nano

type OpenAIAccountScheduleRequest struct {
	GroupID                 *int64
	Platform                string
	SessionHash             string
	StickyAccountID         int64
	StickyPreviousAccountID int64
	StickyWeighted          bool
	SubscriptionPriority    bool
	PreserveStickyBinding   bool
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
	allCandidates             []openAIAccountCandidateScore
	candidates                []openAIAccountCandidateScore
	staleSnapshotCompactRetry []openAIAccountCandidateScore
	selectionOrder            []openAIAccountCandidateScore
	candidateCount            int
	topK                      int
	loadSkew                  float64
	scoringMode               scheduling.RolloutMode
	shadowComparison          *openAIShadowComparison
}

type openAIShadowComparison struct {
	CandidateIDs         []int64
	EligibleCandidateIDs []int64
	CompactRejectedIDs   []int64
	ScoreDiffCount       int
	LegacyRank           []int64
	SharedRank           []int64
	LegacyTopK           []int64
	SharedTopK           []int64
	LegacySelectionOrder []int64
	SharedSelectionOrder []int64
	SelectionDiff        bool
	HardGateViolation    bool
	AffinityEscape       bool
	Fallback             bool
	FallbackReason       string
}

type openAIShadowRuntimeSignal struct {
	AccountID          int64
	ErrorRate          float64
	ErrorRateAvailable bool
	TTFTMillis         float64
	TTFTAvailable      bool
}

type openAIAccountLoadSelectionAttempt struct {
	result              *AccountSelectionResult
	selectionOrder      []openAIAccountCandidateScore
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
	service       *OpenAIGatewayService
	metrics       openAIAccountSchedulerMetrics
	stats         *openAIAccountRuntimeStats
	shadowMetrics schedulerShadowCounters
}

type openAIStickyEscapeConfig struct {
	enabled   bool
	ttftMs    float64
	errorRate float64
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

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID != "" && normalizeOpenAICompatiblePlatform(req.Platform) == PlatformOpenAI &&
		(!req.StickyWeighted || !req.PreviousResponseCanMove) {
		selection, err := s.service.selectAccountByPreviousResponseIDForCapability(
			ctx,
			req.GroupID,
			previousResponseID,
			req.RequestedModel,
			req.ExcludedIDs,
			req.RequiredCapability,
			req.RequireCompact,
		)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			if !s.isAccountTransportCompatible(selection.Account, req.RequiredTransport) {
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				selection = nil
			}
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerPreviousResponse
			decision.StickyPreviousHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, selection.Account.ID)
				s.service.setStickySessionAccountEpoch(ctx, req.GroupID, req.SessionHash, selection.Account, s.service.openAIWSSessionStickyTTL())
			}
			return selection, decision, nil
		}
	}

	// 用户级钉号：同一本地 API Key 固定走同一台上游账号，直到硬失败才换。
	// 优先级在 previous_response（续链必须跟原号）之后、会话粘性之前。
	// 注意：这里只做硬失败判断（排除/不可调度/能力不匹配），不做用量分、TTFT、
	// 并发满之类的软换号——那些正是把同一用户洒到多台账号上的来源。
	if req.UserPinAccountID > 0 && req.APIKeyID > 0 {
		if selection, brokenReason := s.selectByUserPin(ctx, req); selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerUserPin
			decision.UserPinHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			s.service.refreshUserPinTTL(ctx, req.GroupID, req.APIKeyID)
			return selection, decision, nil
		} else if brokenReason != "" {
			decision.UserPinBroken = brokenReason
			s.service.deleteUserPin(ctx, req.GroupID, req.APIKeyID)
			slog.Info("openai_user_pin_broken",
				"api_key_id", req.APIKeyID,
				"account_id", req.UserPinAccountID,
				"reason", brokenReason,
			)
		}
	}

	if !req.StickyWeighted {
		selection, escapedSticky, err := s.selectBySessionHash(ctx, req)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerSessionSticky
			decision.StickySessionHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			return selection, decision, nil
		}
		if escapedSticky {
			req.PreserveStickyBinding = true
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
		if req.StickyWeighted {
			if req.StickyPreviousAccountID > 0 && selection.Account.ID == req.StickyPreviousAccountID {
				decision.StickyPreviousHit = true
			}
			if req.StickyAccountID > 0 && selection.Account.ID == req.StickyAccountID {
				decision.StickySessionHit = true
			}
		}
	}
	return selection, decision, nil
}

// selectByUserPin 尝试命中用户级钉号。只认硬失败（已在排除集 / 拉不到账号 /
// 平台或能力不匹配 / 不可调度），不触发会话粘性那套用量分/TTFT/并发软逃逸。
// 返回 (nil, "") 表示无钉可查（静默走后续层）；(nil, reason) 表示钉住但已硬失败，
// 调用方据此拆钉；(selection, _) 表示命中。
func (s *defaultOpenAIAccountScheduler) selectByUserPin(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, string) {
	if s == nil || s.service == nil {
		return nil, ""
	}
	pinnedID := req.UserPinAccountID
	if pinnedID <= 0 || req.APIKeyID <= 0 {
		return nil, ""
	}
	if scheduling.IsExcluded(req.ExcludedIDs, pinnedID) {
		// failover 已证明这台号对本请求不可用：硬失败，拆钉。
		return nil, "excluded_by_failover"
	}
	account, err := s.service.getSchedulableAccount(ctx, pinnedID)
	if err != nil || account == nil {
		return nil, "account_unavailable"
	}
	if !account.IsOpenAICompatible() || account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) {
		return nil, "platform_mismatch"
	}
	if !account.IsSchedulable() {
		return nil, "unschedulable"
	}
	if !openAIStickyAccountMatchesGroup(account, req.GroupID) {
		return nil, "group_mismatch"
	}
	if !OpenAICredentialClassMatches(account, req.RequiredCredentialClass) {
		return nil, "credential_class_mismatch"
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		return nil, "transport_mismatch"
	}
	if !s.isAccountRequestCompatible(ctx, account, req) {
		// 覆盖模型能力 / 配额窗 / 渠道策略等硬约束（含额度尽、codex_cli_only 类拒绝）。
		return nil, "request_incompatible"
	}

	// 并发满不逃逸：要么等这台（WaitPlan），要么模型预算满直接让路。
	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, req.RequestedModel, account.ID, account.Concurrency)
	if acquireErr == nil && result != nil && result.Acquired {
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, ""
	}
	if acquireErr == nil && result != nil && result.ModelLimited {
		return &AccountSelectionResult{
			Account:      account,
			ModelLimited: true,
		}, ""
	}
	if s.service.concurrencyService != nil {
		cfg := s.service.schedulingConfig()
		return &AccountSelectionResult{
			Account:  account,
			WaitPlan: newStickyWaitPlan(account.ID, account.Concurrency, cfg),
		}, ""
	}
	return nil, "slot_unavailable"
}

func (s *defaultOpenAIAccountScheduler) selectBySessionHash(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, bool, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" || s == nil || s.service == nil || s.service.cache == nil {
		return nil, false, nil
	}

	accountID := req.StickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil, false, nil
		}
	}
	binding := scheduling.EvaluateStickyBinding(sessionHash, accountID, req.ExcludedIDs)
	if !binding.Ready() {
		return nil, false, nil
	}

	account, err := s.service.getSchedulableAccount(ctx, accountID)
	candidateID := int64(0)
	if account != nil {
		candidateID = account.ID
	}
	if err != nil || scheduling.EvaluateStickyCandidate(binding, candidateID, false) != scheduling.StickyReady {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	clearCandidate := shouldClearStickySession(account, req.RequestedModel) || account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) || !account.IsOpenAICompatible() || !account.IsSchedulable()
	if scheduling.EvaluateStickyCandidate(binding, account.ID, clearCandidate) == scheduling.StickyClear {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	if !OpenAICredentialClassMatches(account, req.RequiredCredentialClass) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	if !s.service.stickySessionEpochMatches(ctx, req.GroupID, sessionHash, account) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	if !s.isAccountRequestCompatible(ctx, account, req) {
		return nil, false, nil
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
	if account == nil || !openAIStickyAccountMatchesGroup(account, req.GroupID) || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, false, nil
	}
	// Interval-gated quota re-eval: if bound account is hot and a cooler same-class account exists, escape sticky.
	if s.shouldReevalStickyForUsage(ctx, req, account) {
		slog.Info("sticky_reeval_switch",
			"account_id", account.ID,
			"usage_score", ComputeOpenAIUsageScore(account, time.Now()),
			"session_hash", sessionHash,
		)
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, true, nil
	}
	escapeCfg := s.service.openAIStickyEscapeConfig()
	if reason, errorRate, ttft, shouldEscape := s.shouldEscapeStickyAccount(accountID, escapeCfg); shouldEscape {
		slog.Info("sticky_escape_triggered",
			"account_id", accountID,
			"reason", reason,
			"error_rate", errorRate,
			"ttft", ttft,
		)
		return nil, true, nil
	}
	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, req.RequestedModel, accountID, account.Concurrency)
	if acquireErr == nil && result != nil && result.Acquired {
		_ = s.service.refreshStickySessionTTL(ctx, req.GroupID, sessionHash, s.service.openAIWSSessionStickyTTL())
		s.service.setStickySessionAccountEpoch(ctx, req.GroupID, sessionHash, account, s.service.openAIWSSessionStickyTTL())
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, false, nil
	}
	if acquireErr == nil && result != nil && result.ModelLimited {
		// 模型级并发预算已满：立即让路，不进入账号等待队列。
		return &AccountSelectionResult{
			Account:      account,
			ModelLimited: true,
		}, false, nil
	}

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	if s.service.concurrencyService != nil {
		if escapeCfg.enabled && acquireErr == nil && result != nil && !result.Acquired {
			errorRate, ttft, _ := s.stats.snapshot(accountID)
			slog.Info("sticky_escape_triggered",
				"account_id", accountID,
				"reason", "concurrency_full",
				"error_rate", errorRate,
				"ttft", ttft,
			)
			return nil, true, nil
		}
		return &AccountSelectionResult{
			Account:  account,
			WaitPlan: newStickyWaitPlan(accountID, account.Concurrency, cfg),
		}, false, nil
	}
	return nil, false, nil
}

func (s *defaultOpenAIAccountScheduler) shouldReevalStickyForUsage(ctx context.Context, req OpenAIAccountScheduleRequest, bound *Account) bool {
	if s == nil || s.service == nil || bound == nil {
		return false
	}
	now := time.Now()
	sessionKey := strings.TrimSpace(req.SessionHash)
	if sessionKey != "" {
		if raw, ok := stickyReevalLastAt.Load(sessionKey); ok {
			if lastNano, ok := raw.(int64); ok {
				if now.Sub(time.Unix(0, lastNano)) < openAIStickyReevalInterval {
					return false
				}
			}
		}
		stickyReevalLastAt.Store(sessionKey, now.UnixNano())
	}
	boundScore := ComputeOpenAIUsageScore(bound, now)
	if boundScore < openAIStickyAutoSwitchPct {
		return false
	}
	// Look for any eligible cooler account in the same group/platform/class.
	// Best-effort: use snapshot list when available; otherwise skip.
	if s.service.schedulerSnapshot == nil {
		return false
	}
	accounts, _, err := s.service.schedulerSnapshot.ListSchedulableAccounts(ctx, req.GroupID, normalizeOpenAICompatiblePlatform(req.Platform), false)
	if err != nil || len(accounts) == 0 {
		return false
	}
	for i := range accounts {
		cand := &accounts[i]
		if cand.ID == bound.ID {
			continue
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[cand.ID]; excluded {
				continue
			}
		}
		if !cand.IsSchedulable() || !cand.IsOpenAICompatible() {
			continue
		}
		if !OpenAICredentialClassMatches(cand, req.RequiredCredentialClass) && !OpenAICredentialClassMatches(cand, OpenAIAccountCredentialClass(bound)) {
			continue
		}
		if !s.isAccountRequestCompatible(ctx, cand, req) {
			continue
		}
		score := ComputeOpenAIUsageScore(cand, now)
		if score < boundScore {
			return true
		}
	}
	return false
}

func openAIStickyAccountMatchesGroup(account *Account, groupID *int64) bool {
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{GroupID: groupID, IncludeFlatGroupIDs: true},
		scheduling.GroupMembershipPolicy,
	).Allowed
}

func openAIAccountSchedulingPriority(account *Account) int {
	if account == nil {
		return 0
	}
	return account.Priority
}

func (s *defaultOpenAIAccountScheduler) shouldEscapeStickyAccount(accountID int64, cfg openAIStickyEscapeConfig) (reason string, errorRate float64, ttft float64, shouldEscape bool) {
	if !cfg.enabled || s == nil || s.stats == nil || accountID <= 0 {
		return "", 0, 0, false
	}
	errorRate, ttft, hasTTFT := s.stats.snapshot(accountID)
	if hasTTFT && ttft > cfg.ttftMs {
		return "ttft", errorRate, ttft, true
	}
	if errorRate > cfg.errorRate {
		return "error_rate", errorRate, ttft, true
	}
	return "", errorRate, ttft, false
}

type openAIAccountCandidateScore struct {
	account              *Account
	loadInfo             *AccountLoadInfo
	score                float64
	priority             int
	errorRate            float64
	hasErrorRate         bool
	ttft                 float64
	hasTTFT              bool
	compactErrorRate     float64
	compactLatencyMs     float64
	hasCompactLatencyMs  bool
	compactPenaltyActive bool
}

func deriveOpenAISelectionSeed(req OpenAIAccountScheduleRequest) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0})
	}

	writeValue(req.SessionHash)
	writeValue(req.PreviousResponseID)
	writeValue(req.RequestedModel)
	if req.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*req.GroupID, 10)))
	}

	seed := hasher.Sum64()
	// 对“无会话锚点”的纯负载均衡请求引入时间熵，避免固定命中同一账号。
	if strings.TrimSpace(req.SessionHash) == "" && strings.TrimSpace(req.PreviousResponseID) == "" {
		seed ^= uint64(time.Now().UnixNano())
	}
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
	}
	return seed
}

func scoredOpenAIAccountCandidates(candidates []openAIAccountCandidateScore) []scheduling.ScoredCandidate {
	result := make([]scheduling.ScoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, scheduling.ScoredCandidate{
			Candidate: scheduling.CandidateSnapshot{
				AccountID: candidate.account.ID,
				Priority:  candidate.priority,
				Signals: scheduling.CandidateSignals{
					LoadRate:   scheduling.AvailableSignal(float64(candidate.loadInfo.LoadRate), time.Time{}),
					QueueDepth: scheduling.AvailableSignal(float64(candidate.loadInfo.WaitingCount), time.Time{}),
				},
			},
			Breakdown: scheduling.ScoreBreakdown{Total: candidate.score},
		})
	}
	return result
}

func openAIAccountCandidatesInSharedOrder(
	ordered []scheduling.ScoredCandidate,
	candidates []openAIAccountCandidateScore,
) []openAIAccountCandidateScore {
	byID := make(map[int64]openAIAccountCandidateScore, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.account.ID] = candidate
	}
	result := make([]openAIAccountCandidateScore, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, byID[candidate.Candidate.AccountID])
	}
	return result
}

func selectTopKOpenAICandidatesShared(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	return openAIAccountCandidatesInSharedOrder(
		scheduling.SelectTopK(scoredOpenAIAccountCandidates(candidates), topK),
		candidates,
	)
}

func buildOpenAIWeightedSelectionOrderShared(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidateScore {
	return buildOpenAIWeightedSelectionOrderSharedWithSeed(candidates, deriveOpenAISelectionSeed(req))
}

func buildOpenAIWeightedSelectionOrderSharedWithSeed(
	candidates []openAIAccountCandidateScore,
	seed uint64,
) []openAIAccountCandidateScore {
	rng := scheduling.NewSelectionRNG(seed)
	return openAIAccountCandidatesInSharedOrder(
		scheduling.BuildWeightedSelectionOrder(scoredOpenAIAccountCandidates(candidates), &rng),
		candidates,
	)
}

// hasOpenAICompactFailureSignals is true only when some candidate has compact
// error history. Successful latency alone must not disable weighted exploration.
func hasOpenAICompactFailureSignals(candidates []openAIAccountCandidateScore) bool {
	for _, candidate := range candidates {
		if candidate.compactPenaltyActive || candidate.compactErrorRate > 0 {
			return true
		}
	}
	return false
}

func openAIResetNormalizationRange(
	candidates []openAIAccountCandidateScore,
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	now time.Time,
) *scheduling.NormalizationRange {
	if weights.Reset <= 0 {
		return nil
	}

	result := &scheduling.NormalizationRange{}
	for _, candidate := range candidates {
		end := candidate.account.SessionWindowEnd
		if end == nil || !now.Before(*end) {
			continue
		}
		remaining := end.Sub(now).Seconds()
		if !result.Available {
			result.Min, result.Max, result.Available = remaining, remaining, true
			continue
		}
		if remaining < result.Min {
			result.Min = remaining
		}
		if remaining > result.Max {
			result.Max = remaining
		}
	}
	return result
}

func scoreOpenAIAccountCandidatesShared(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	resetRange *scheduling.NormalizationRange,
	now time.Time,
) []openAIAccountCandidateScore {
	snapshots := make([]scheduling.CandidateSnapshot, 0, len(candidates))
	for _, candidate := range candidates {
		snapshot := scheduling.CandidateSnapshot{
			AccountID: candidate.account.ID,
			Platform:  candidate.account.Platform,
			Priority:  candidate.priority,
			Signals: scheduling.CandidateSignals{
				LoadRate:   scheduling.AvailableSignal(float64(candidate.loadInfo.LoadRate), time.Time{}),
				QueueDepth: scheduling.AvailableSignal(float64(candidate.loadInfo.WaitingCount), time.Time{}),
				ErrorRate:  scheduling.AvailableSignal(candidate.errorRate, time.Time{}),
			},
			Affinity: scheduling.AffinitySignals{
				PreviousResponse: req.PreviousResponseCanMove && req.StickyPreviousAccountID > 0 && candidate.account.ID == req.StickyPreviousAccountID,
				SessionSticky:    req.StickyAccountID > 0 && candidate.account.ID == req.StickyAccountID,
			},
		}
		if candidate.hasTTFT {
			snapshot.Signals.TTFTMillis = scheduling.AvailableSignal(candidate.ttft, time.Time{})
		}
		if weights.Reset > 0 {
			if end := candidate.account.SessionWindowEnd; end != nil && now.Before(*end) {
				snapshot.Signals.ResetSeconds = scheduling.AvailableSignal(end.Sub(now).Seconds(), time.Time{})
			}
		}
		if weights.QuotaHeadroom > 0 {
			snapshot.Signals.QuotaHeadroom = scheduling.AvailableSignal(openAIQuotaHeadroomFactor(candidate.account, now), time.Time{})
		}
		if req.RequireCompact {
			snapshot.Signals.CompactError = scheduling.AvailableSignal(candidate.compactErrorRate, time.Time{})
			if candidate.hasCompactLatencyMs {
				snapshot.Signals.CompactMillis = scheduling.AvailableSignal(candidate.compactLatencyMs, time.Time{})
			}
		}
		snapshots = append(snapshots, snapshot)
	}

	scored := scheduling.ScoreCandidates(snapshots, scheduling.ScoreWeights{
		Priority:         weights.Priority,
		Load:             weights.Load,
		Queue:            weights.Queue,
		ErrorRate:        weights.ErrorRate,
		TTFT:             weights.TTFT,
		Reset:            weights.Reset,
		QuotaHeadroom:    weights.QuotaHeadroom,
		CompactError:     weights.CompactError,
		CompactLatency:   weights.CompactLatency,
		PreviousResponse: weights.Previous,
		SessionSticky:    weights.SessionSticky,
	}, scheduling.ScoreOptions{
		RequireCompact: req.RequireCompact,
		StickyWeighted: req.StickyWeighted,
		ResetRange:     resetRange,
	})
	for i := range candidates {
		candidates[i].score = scored[i].Score()
	}
	return candidates
}

func (s *defaultOpenAIAccountScheduler) logOpenAIScorerShadowDiff(
	req OpenAIAccountScheduleRequest,
	plan openAIAccountLoadPlan,
	comparison openAIShadowComparison,
) {
	signals := make([]openAIShadowRuntimeSignal, 0, len(plan.allCandidates))
	for _, candidate := range plan.allCandidates {
		if candidate.account == nil {
			continue
		}
		signals = append(signals, openAIShadowRuntimeSignal{
			AccountID:          candidate.account.ID,
			ErrorRate:          candidate.errorRate,
			ErrorRateAvailable: candidate.hasErrorRate,
			TTFTMillis:         candidate.ttft,
			TTFTAvailable:      candidate.hasTTFT,
		})
	}
	counters := schedulerShadowCountersSnapshot{}
	liveAccountSwitchTotal := int64(0)
	if s != nil {
		counters = s.shadowMetrics.record(
			len(comparison.EligibleCandidateIDs),
			comparison.SelectionDiff,
			comparison.AffinityEscape,
			comparison.HardGateViolation,
			comparison.Fallback,
		)
		liveAccountSwitchTotal = s.metrics.accountSwitchTotal.Load()
	}
	slog.Info("scheduler_scoring_shadow",
		"platform", normalizeOpenAICompatiblePlatform(req.Platform),
		"group_id", derefGroupID(req.GroupID),
		"candidate_count", len(comparison.EligibleCandidateIDs),
		"candidate_ids", comparison.CandidateIDs,
		"eligible_candidate_ids", comparison.EligibleCandidateIDs,
		"compact_rejected_ids", comparison.CompactRejectedIDs,
		"score_diff_count", comparison.ScoreDiffCount,
		"legacy_rank", comparison.LegacyRank,
		"shared_rank", comparison.SharedRank,
		"legacy_top_k", comparison.LegacyTopK,
		"shared_top_k", comparison.SharedTopK,
		"legacy_selection_order", comparison.LegacySelectionOrder,
		"shared_selection_order", comparison.SharedSelectionOrder,
		"selection_diff", comparison.SelectionDiff,
		"hard_gate_violation", comparison.HardGateViolation,
		"affinity_escape", comparison.AffinityEscape,
		"fallback", comparison.Fallback,
		"fallback_reason", comparison.FallbackReason,
		"runtime_signals", signals,
		"evaluation_total", counters.EvaluationTotal,
		"candidate_total", counters.CandidateTotal,
		"selection_diff_total", counters.SelectionDiffTotal,
		"counterfactual_switch_total", counters.SelectionDiffTotal,
		"live_account_switch_total", liveAccountSwitchTotal,
		"affinity_escape_total", counters.AffinityEscapeTotal,
		"hard_gate_violation_total", counters.HardGateViolationTotal,
		"fallback_total", counters.FallbackTotal,
	)
}

func applyOpenAIShadowOutcome(req OpenAIAccountScheduleRequest, comparison *openAIShadowComparison) {
	if comparison == nil {
		return
	}
	sharedSelectedID := firstInt64(comparison.SharedSelectionOrder)
	affinityID := req.StickyPreviousAccountID
	if affinityID <= 0 {
		affinityID = req.StickyAccountID
	}
	comparison.AffinityEscape = affinityID > 0 && sharedSelectedID != affinityID
	comparison.Fallback = sharedSelectedID == 0
	if comparison.Fallback {
		comparison.FallbackReason = "no_counterfactual_selection"
	}
}

func buildOpenAIShadowComparison(plan, sharedPlan openAIAccountLoadPlan) openAIShadowComparison {
	legacyRanked := selectTopKOpenAICandidatesShared(plan.candidates, len(plan.candidates))
	sharedRanked := selectTopKOpenAICandidatesShared(sharedPlan.candidates, len(sharedPlan.candidates))
	scoreDiffs := 0
	for i := range plan.candidates {
		if math.Abs(plan.candidates[i].score-sharedPlan.candidates[i].score) > 1e-12 {
			scoreDiffs++
		}
	}
	eligibleIDs := openAIAccountCandidateIDs(plan.candidates)
	sharedSelectionOrder := openAIAccountCandidateIDs(sharedPlan.selectionOrder)
	return openAIShadowComparison{
		CandidateIDs:         openAIAccountCandidateIDs(plan.allCandidates),
		EligibleCandidateIDs: eligibleIDs,
		CompactRejectedIDs:   openAIAccountCandidateIDs(plan.staleSnapshotCompactRetry),
		ScoreDiffCount:       scoreDiffs,
		LegacyRank:           openAIAccountCandidateIDs(legacyRanked),
		SharedRank:           openAIAccountCandidateIDs(sharedRanked),
		LegacyTopK:           openAIAccountCandidateIDs(selectTopKOpenAICandidatesShared(plan.candidates, plan.topK)),
		SharedTopK:           openAIAccountCandidateIDs(selectTopKOpenAICandidatesShared(sharedPlan.candidates, sharedPlan.topK)),
		LegacySelectionOrder: openAIAccountCandidateIDs(plan.selectionOrder),
		SharedSelectionOrder: sharedSelectionOrder,
		SelectionDiff:        firstOpenAIAccountCandidateID(plan.selectionOrder) != firstOpenAIAccountCandidateID(sharedPlan.selectionOrder),
		HardGateViolation:    hasOpenAIShadowSelectionOutsideEligible(sharedSelectionOrder, eligibleIDs),
	}
}

func hasOpenAIShadowSelectionOutsideEligible(selectionOrder, eligibleIDs []int64) bool {
	eligible := make(map[int64]struct{}, len(eligibleIDs))
	for _, accountID := range eligibleIDs {
		eligible[accountID] = struct{}{}
	}
	for _, accountID := range selectionOrder {
		if _, ok := eligible[accountID]; !ok {
			return true
		}
	}
	return false
}

func openAIAccountCandidateIDs(candidates []openAIAccountCandidateScore) []int64 {
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

func firstOpenAIAccountCandidateID(candidates []openAIAccountCandidateScore) int64 {
	if len(candidates) == 0 || candidates[0].account == nil {
		return 0
	}
	return candidates[0].account.ID
}

func (s *defaultOpenAIAccountScheduler) buildOpenAIAccountLoadPlan(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	filtered []*Account,
	loadMap map[int64]*AccountLoadInfo,
) openAIAccountLoadPlan {
	allCandidates := make([]openAIAccountCandidateScore, 0, len(filtered))
	for _, account := range filtered {
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		errorRate, ttft, hasTTFT := 0.0, 0.0, false
		compactErrorRate, compactLatencyMs, hasCompactLatencyMs, compactPenaltyActive := 0.0, 0.0, false, false
		if s.stats != nil {
			errorRate, ttft, hasTTFT = s.stats.snapshot(account.ID)
			if req.RequireCompact {
				compactErrorRate, compactLatencyMs, hasCompactLatencyMs, compactPenaltyActive = s.stats.snapshotCompact(account.ID)
			}
		}
		allCandidates = append(allCandidates, openAIAccountCandidateScore{
			account:              account,
			loadInfo:             loadInfo,
			errorRate:            errorRate,
			hasErrorRate:         s.stats != nil && s.stats.has(account.ID),
			ttft:                 ttft,
			hasTTFT:              hasTTFT,
			compactErrorRate:     compactErrorRate,
			compactLatencyMs:     compactLatencyMs,
			hasCompactLatencyMs:  hasCompactLatencyMs,
			compactPenaltyActive: compactPenaltyActive,
		})
	}

	candidates := allCandidates
	staleSnapshotCompactRetry := make([]openAIAccountCandidateScore, 0, len(allCandidates))
	if req.RequireCompact {
		candidates = make([]openAIAccountCandidateScore, 0, len(allCandidates))
		for _, candidate := range allCandidates {
			if openAICompactSupportTier(candidate.account) == 0 {
				staleSnapshotCompactRetry = append(staleSnapshotCompactRetry, candidate)
				continue
			}
			candidates = append(candidates, candidate)
		}
	}

	plan := openAIAccountLoadPlan{
		allCandidates:             allCandidates,
		candidates:                candidates,
		staleSnapshotCompactRetry: staleSnapshotCompactRetry,
		candidateCount:            len(candidates),
	}
	mode := s.service.schedulerScoringRolloutMode(ctx, normalizeOpenAICompatiblePlatform(req.Platform), req.GroupID)
	plan.scoringMode = mode
	if len(candidates) == 0 {
		plan.selectionOrder = s.buildOpenAISelectionOrder(req, plan)
		if mode == scheduling.RolloutShadow {
			sharedPlan := plan
			sharedPlan.scoringMode = scheduling.RolloutEnforce
			comparison := buildOpenAIShadowComparison(plan, sharedPlan)
			applyOpenAIShadowOutcome(req, &comparison)
			plan.shadowComparison = &comparison
			s.logOpenAIScorerShadowDiff(req, plan, comparison)
		}
		return plan
	}

	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	for i := range candidates {
		candidate := &candidates[i]
		candidate.priority = openAIAccountSchedulingPriority(candidate.account)
		loadRate := float64(candidate.loadInfo.LoadRate)
		loadRateSum += loadRate
		loadRateSumSquares += loadRate * loadRate
	}
	plan.loadSkew = calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(candidates))

	weights := s.service.openAIWSSchedulerWeightsForRequest(ctx)

	resetRange := openAIResetNormalizationRange(candidates, weights, time.Now())
	now := time.Now()
	candidates = scoreOpenAIAccountCandidatesShared(candidates, req, weights, resetRange, now)
	plan.candidates = candidates

	plan.topK = s.service.openAIWSLBTopKForRequest(ctx)
	if plan.topK > len(candidates) {
		plan.topK = len(candidates)
	}
	if plan.topK <= 0 {
		plan.topK = 1
	}

	sortOpenAIFreshSessionCandidates(plan.candidates, req, now)
	selectionSeed := deriveOpenAISelectionSeed(req)
	plan.selectionOrder = s.buildOpenAISelectionOrderWithSeed(req, plan, selectionSeed)
	if mode == scheduling.RolloutShadow {
		sharedPlan := plan
		sharedPlan.scoringMode = scheduling.RolloutEnforce
		comparison := buildOpenAIShadowComparison(plan, sharedPlan)
		applyOpenAIShadowOutcome(req, &comparison)
		plan.shadowComparison = &comparison
		s.logOpenAIScorerShadowDiff(req, plan, comparison)
	}
	return plan
}

func sortOpenAIFreshSessionCandidates(candidates []openAIAccountCandidateScore, req OpenAIAccountScheduleRequest, now time.Time) {
	if req.StickyWeighted || req.StickyAccountID > 0 || req.StickyPreviousAccountID > 0 || strings.TrimSpace(req.PreviousResponseID) != "" || strings.TrimSpace(req.SessionHash) != "" {
		return
	}
	reqModel := strings.TrimSpace(req.RequestedModel)
	sort.SliceStable(candidates, func(i, j int) bool {
		// 按请求 model 对应池槽位判断：astra 请求只认 luna 槽的票。
		ri := accountHasOpenAICodexTurnStateReserveForModel(candidates[i].account, reqModel, now, openAICodexTurnStateDefaultTTL)
		rj := accountHasOpenAICodexTurnStateReserveForModel(candidates[j].account, reqModel, now, openAICodexTurnStateDefaultTTL)
		if ri != rj {
			return ri
		}
		si := ComputeOpenAIUsageScore(candidates[i].account, now)
		sj := ComputeOpenAIUsageScore(candidates[j].account, now)
		if si != sj {
			return si < sj
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		idI, idJ := int64(0), int64(0)
		if candidates[i].account != nil {
			idI = candidates[i].account.ID
		}
		if candidates[j].account != nil {
			idJ = candidates[j].account.ID
		}
		return idI < idJ
	})
}

func (s *defaultOpenAIAccountScheduler) buildOpenAISelectionOrder(
	req OpenAIAccountScheduleRequest,
	plan openAIAccountLoadPlan,
) []openAIAccountCandidateScore {
	return s.buildOpenAISelectionOrderWithSeed(req, plan, deriveOpenAISelectionSeed(req))
}

func (s *defaultOpenAIAccountScheduler) buildOpenAISelectionOrderWithSeed(
	req OpenAIAccountScheduleRequest,
	plan openAIAccountLoadPlan,
	seed uint64,
) []openAIAccountCandidateScore {
	buildSelectionOrder := func(pool []openAIAccountCandidateScore) []openAIAccountCandidateScore {
		if len(pool) == 0 || plan.topK <= 0 {
			return nil
		}
		groupTopK := plan.topK
		if groupTopK > len(pool) {
			groupTopK = len(pool)
		}
		ranked := selectTopKOpenAICandidatesShared(pool, groupTopK)
		if req.StickyWeighted {
			for _, stickyID := range []int64{req.StickyPreviousAccountID, req.StickyAccountID} {
				if stickyID <= 0 {
					continue
				}
				for i, candidate := range ranked {
					if candidate.account != nil && candidate.account.ID == stickyID {
						ordered := append([]openAIAccountCandidateScore{candidate}, ranked[:i]...)
						ordered = append(ordered, ranked[i+1:]...)
						return ordered
					}
				}
			}
		}
		// Only force deterministic ranking when compact failures exist; pure
		// success/latency history should still explore via weighted selection.
		if req.RequireCompact && hasOpenAICompactFailureSignals(ranked) {
			return ranked
		}
		// Fresh sessions: keep lowest-usage order from buildLoadPlan (not score topK).
		if !req.StickyWeighted && req.StickyAccountID <= 0 && req.StickyPreviousAccountID <= 0 && strings.TrimSpace(req.PreviousResponseID) == "" && strings.TrimSpace(req.SessionHash) == "" {
			if groupTopK >= len(pool) {
				return append([]openAIAccountCandidateScore(nil), pool...)
			}
			return append([]openAIAccountCandidateScore(nil), pool[:groupTopK]...)
		}
		return buildOpenAIWeightedSelectionOrderSharedWithSeed(ranked, seed)
	}

	if req.RequireCompact {
		supported := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		unknown := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		supportedPenalized := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		unknownPenalized := make([]openAIAccountCandidateScore, 0, len(plan.candidates))
		for _, candidate := range plan.candidates {
			switch openAICompactSupportTier(candidate.account) {
			case 2:
				if candidate.compactPenaltyActive {
					supportedPenalized = append(supportedPenalized, candidate)
				} else {
					supported = append(supported, candidate)
				}
			case 1:
				if candidate.compactPenaltyActive {
					unknownPenalized = append(unknownPenalized, candidate)
				} else {
					unknown = append(unknown, candidate)
				}
			}
		}
		selectionOrder := make([]openAIAccountCandidateScore, 0, len(plan.allCandidates))
		selectionOrder = append(selectionOrder, buildSelectionOrder(supported)...)
		selectionOrder = append(selectionOrder, buildSelectionOrder(unknown)...)
		selectionOrder = append(selectionOrder, buildSelectionOrder(supportedPenalized)...)
		selectionOrder = append(selectionOrder, buildSelectionOrder(unknownPenalized)...)
		if len(plan.staleSnapshotCompactRetry) > 0 && s.service.schedulerSnapshot != nil {
			selectionOrder = append(selectionOrder, sortOpenAICompactRetryCandidates(plan.staleSnapshotCompactRetry)...)
		}
		return selectionOrder
	}

	return buildSelectionOrder(plan.candidates)
}

func sortOpenAICompactRetryCandidates(pool []openAIAccountCandidateScore) []openAIAccountCandidateScore {
	if len(pool) == 0 {
		return nil
	}
	ordered := append([]openAIAccountCandidateScore(nil), pool...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.account.Priority != b.account.Priority {
			return a.account.Priority < b.account.Priority
		}
		if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
			return a.loadInfo.LoadRate < b.loadInfo.LoadRate
		}
		if a.loadInfo.WaitingCount != b.loadInfo.WaitingCount {
			return a.loadInfo.WaitingCount < b.loadInfo.WaitingCount
		}
		switch {
		case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
			return true
		case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
			return false
		case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
			return false
		default:
			return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
		}
	})
	return ordered
}

func (s *defaultOpenAIAccountScheduler) tryAcquireOpenAISelectionOrder(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	selectionOrder []openAIAccountCandidateScore,
) (*AccountSelectionResult, bool, error) {
	compactBlocked := false
	for i := 0; i < len(selectionOrder); i++ {
		candidate := selectionOrder[i]
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
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
			if req.SessionHash != "" && !req.PreserveStickyBinding {
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

func (s *defaultOpenAIAccountScheduler) tryFallbackToWeightedSticky(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, error) {
	if !req.StickyWeighted {
		return nil, nil
	}
	for _, accountID := range []int64{req.StickyPreviousAccountID, req.StickyAccountID} {
		if accountID <= 0 {
			continue
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[accountID]; excluded {
				continue
			}
		}
		account, err := s.service.getSchedulableAccount(ctx, accountID)
		if err != nil || account == nil {
			continue
		}
		if !s.isAccountRequestCompatible(ctx, account, req) || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
		if account == nil || !s.isAccountRequestCompatible(ctx, account, req) || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		// 粘性绑定只证明绑定时账号在分组内；账号被移出分组后绑定仍会在 TTL 内存活，
		// 必须与 selectBySessionHash 一样重验分组归属，否则会把分组流量泄漏到组外账号。
		if !openAIStickyAccountMatchesGroup(account, req.GroupID) {
			if accountID == req.StickyAccountID && strings.TrimSpace(req.SessionHash) != "" {
				_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
			}
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(account) == 0 {
			continue
		}
		result, acquireErr := s.service.tryAcquireAccountSlot(ctx, req.RequestedModel, account.ID, account.Concurrency)
		if acquireErr != nil {
			return nil, acquireErr
		}
		if result != nil && result.Acquired {
			if req.SessionHash != "" && !req.PreserveStickyBinding {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, account.ID)
			}
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
			}, nil
		}
		if result != nil && result.ModelLimited {
			// 模型级并发预算已满：立即让路，不进入账号等待队列。
			return &AccountSelectionResult{
				Account:      account,
				ModelLimited: true,
			}, nil
		}
		if s.service.concurrencyService != nil {
			cfg := s.service.schedulingConfig()
			return &AccountSelectionResult{
				Account:  account,
				WaitPlan: newStickyWaitPlan(account.ID, account.Concurrency, cfg),
			}, nil
		}
	}
	return nil, nil
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
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByID(ctx, *req.GroupID)
	}

	filtered := make([]*Account, 0, len(accounts))
	loadReq := make([]AccountWithConcurrency, 0, len(accounts))
	shadowPolicyEnabled := s.service.schedulerScoringRolloutMode(ctx, normalizeOpenAICompatiblePlatform(req.Platform), req.GroupID) == scheduling.RolloutShadow
	shadowPolicyTrace := make([]scheduling.CandidatePolicyTrace, 0, len(accounts))
	recordShadowPolicy := func(account *Account, allowed bool, rejectedPolicy string) {
		if !shadowPolicyEnabled {
			return
		}
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
		}
		shadowPolicyTrace = append(shadowPolicyTrace, scheduling.CandidatePolicyTrace{
			AccountID:      accountID,
			Allowed:        allowed,
			RejectedPolicy: rejectedPolicy,
		})
	}
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				recordShadowPolicy(account, false, "not_excluded")
				continue
			}
		}
		if !account.IsSchedulable() {
			recordShadowPolicy(account, false, "schedulable")
			continue
		}
		if account.Platform != normalizeOpenAICompatiblePlatform(req.Platform) {
			recordShadowPolicy(account, false, "platform")
			continue
		}
		if !account.IsOpenAICompatible() {
			recordShadowPolicy(account, false, "openai_compatible")
			continue
		}
		if !OpenAICredentialClassMatches(account, req.RequiredCredentialClass) {
			recordShadowPolicy(account, false, "credential_class")
			continue
		}
		if s.service.isOpenAIAccountRuntimeBlocked(account) {
			recordShadowPolicy(account, false, "runtime_blocked")
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			recordShadowPolicy(account, false, "privacy")
			s.service.BlockAccountScheduling(account, time.Time{}, "privacy_not_set")
			_ = s.service.accountRepo.SetError(ctx, account.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		requestDecision := s.accountRequestPolicyDecision(ctx, account, req)
		if !requestDecision.Allowed {
			recordShadowPolicy(account, false, requestDecision.RejectedPolicy)
			continue
		}
		transportDecision := scheduling.EvaluateCandidate(
			context.Background(),
			account,
			scheduling.Request{RequiredTransport: string(req.RequiredTransport)},
			openAITransportPolicy{service: s.service},
		)
		if !transportDecision.Allowed {
			recordShadowPolicy(account, false, transportDecision.RejectedPolicy)
			continue
		}
		recordShadowPolicy(account, true, "")
		filtered = append(filtered, account)
		loadReq = append(loadReq, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
		})
	}
	if shadowPolicyEnabled {
		slog.Info("scheduler_scoring_shadow_policy",
			"platform", normalizeOpenAICompatiblePlatform(req.Platform),
			"group_id", derefGroupID(req.GroupID),
			"raw_candidate_count", len(accounts),
			"allowed_candidate_ids", openAIAccountPointerIDs(filtered),
			"policy_trace", shadowPolicyTrace,
		)
	}
	if len(filtered) == 0 {
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false)
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

	if req.SubscriptionPriority {
		subscriptionAccounts, regularAccounts := partitionOpenAIChatGPTSubscriptionAccounts(filtered)
		if len(subscriptionAccounts) > 0 {
			attempt := s.trySelectByLoadBalancePool(ctx, req, subscriptionAccounts, loadMap)
			if attempt.err != nil && (!attempt.noCompactCandidates || len(regularAccounts) <= 0) {
				return nil, attempt.candidateCount, attempt.topK, attempt.loadSkew, attempt.err
			}
			if attempt.result != nil {
				return attempt.result, attempt.candidateCount, attempt.topK, attempt.loadSkew, nil
			}
			if len(regularAccounts) > 0 {
				regularAttempt := s.trySelectByLoadBalancePool(ctx, req, regularAccounts, loadMap)
				if regularAttempt.err != nil && !regularAttempt.noCompactCandidates {
					return nil, regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew, regularAttempt.err
				}
				if regularAttempt.result != nil {
					return regularAttempt.result, regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew, nil
				}
				var result *AccountSelectionResult
				candidateCount, topK, loadSkew := regularAttempt.candidateCount, regularAttempt.topK, regularAttempt.loadSkew
				fallbackErr := regularAttempt.err
				if regularAttempt.err == nil {
					result, candidateCount, topK, loadSkew, fallbackErr = s.finishLoadBalanceSelectionFallback(ctx, req, regularAttempt)
					if fallbackErr == nil && result != nil {
						return result, candidateCount, topK, loadSkew, nil
					}
				}
				// 常规池既无法获取也无法排队（含仅剩不支持 compact 的候选）时，
				// 回退到订阅池的等待计划：busy-but-waitable 的订阅账号不应因常规池存在
				// 而被丢弃，否则开启订阅优先反而让本可排队成功的请求硬失败。
				subResult, subCandidateCount, subTopK, subLoadSkew, subErr := s.finishLoadBalanceSelectionFallback(ctx, req, attempt)
				if subErr == nil && subResult != nil {
					return subResult, subCandidateCount, subTopK, subLoadSkew, nil
				}
				return result, candidateCount, topK, loadSkew, fallbackErr
			}
			return s.finishLoadBalanceSelectionFallback(ctx, req, attempt)
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

func partitionOpenAIChatGPTSubscriptionAccounts(accounts []*Account) ([]*Account, []*Account) {
	subscriptionAccounts := make([]*Account, 0, len(accounts))
	regularAccounts := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil && account.IsOpenAIChatGPTSubscription() {
			subscriptionAccounts = append(subscriptionAccounts, account)
			continue
		}
		regularAccounts = append(regularAccounts, account)
	}
	return subscriptionAccounts, regularAccounts
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
	if req.RequireCompact && len(plan.candidates) == 0 && len(plan.staleSnapshotCompactRetry) == 0 {
		attempt.noCompactCandidates = true
		attempt.err = ErrNoAvailableCompactAccounts
		return attempt
	}
	if req.RequireCompact && len(attempt.selectionOrder) == 0 && s.service.schedulerSnapshot == nil {
		attempt.noCompactCandidates = true
		attempt.err = ErrNoAvailableCompactAccounts
		return attempt
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
			MaxConcurrency: account.EffectiveLoadFactor(),
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
		return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, attempt.compactBlocked)
	}

	if stickyFallback, stickyErr := s.tryFallbackToWeightedSticky(ctx, req); stickyErr != nil {
		return nil, candidateCount, topK, loadSkew, stickyErr
	} else if stickyFallback != nil {
		return stickyFallback, candidateCount, topK, loadSkew, nil
	}

	cfg := s.service.schedulingConfig()
	compactBlocked := attempt.compactBlocked
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	for _, candidate := range attempt.selectionOrder {
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
			continue
		}
		fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.Platform, req.RequestedModel, false, req.RequiredCapability)
		if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(ctx, fresh, req) {
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

	return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
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

func (s *OpenAIGatewayService) openAIAdvancedSchedulerSettingService() *SettingService {
	if s == nil {
		return nil
	}
	if s.settingService != nil {
		return s.settingService
	}
	if s.rateLimitService != nil {
		return s.rateLimitService.settingService
	}
	return nil
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerSettingRepo() SettingRepository {
	if settingSvc := s.openAIAdvancedSchedulerSettingService(); settingSvc != nil {
		return settingSvc.settingRepo
	}
	return nil
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerRuntimeSettings(ctx context.Context) openAIAdvancedSchedulerRuntimeSettings {
	settingSvc := s.openAIAdvancedSchedulerSettingService()
	if settingSvc != nil {
		if cached, ok := settingSvc.openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return openAIAdvancedSchedulerRuntimeSettings{
					enabled:                     cached.enabled,
					stickyWeightedEnabled:       cached.stickyWeightedEnabled,
					subscriptionPriorityEnabled: cached.subscriptionPriorityEnabled,
					lbTopKOverride:              cached.lbTopKOverride,
					weightOverrides:             cloneOpenAIAdvancedSchedulerWeightOverrides(cached.weightOverrides),
				}
			}
		}
	}

	doLoad := func() (any, error) {
		if settingSvc != nil {
			if cached, ok := settingSvc.openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
				if time.Now().UnixNano() < cached.expiresAt {
					return openAIAdvancedSchedulerRuntimeSettings{
						enabled:                     cached.enabled,
						stickyWeightedEnabled:       cached.stickyWeightedEnabled,
						subscriptionPriorityEnabled: cached.subscriptionPriorityEnabled,
						lbTopKOverride:              cached.lbTopKOverride,
						weightOverrides:             cloneOpenAIAdvancedSchedulerWeightOverrides(cached.weightOverrides),
					}, nil
				}
			}
		}

		enabled := false
		stickyWeightedEnabled := false
		subscriptionPriorityEnabled := false
		lbTopKOverride := 0
		weightOverrides := map[string]float64{}
		if repo := s.openAIAdvancedSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdvancedSchedulerSettingDBTimeout)
			defer cancel()

			if values, err := repo.GetMultiple(dbCtx, openAIAdvancedSchedulerRuntimeSettingKeys()); err == nil {
				enabled = strings.EqualFold(strings.TrimSpace(values[openAIAdvancedSchedulerSettingKey]), "true")
				stickyWeightedEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled]), "true")
				subscriptionPriorityEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled]), "true")
				lbTopKOverride = parsePositiveIntOverride(values[SettingKeyOpenAIAdvancedSchedulerLBTopK])
				weightOverrides = parseOpenAIAdvancedSchedulerWeightOverrides(values)
			} else {
				// 批量读取失败时逐键降级，覆盖全部键（含 TopK/权重），避免只加载布尔开关
				// 而静默丢弃管理员配置的覆盖值；降级状态会被缓存一个 TTL，必须留痕。
				slog.Warn("openai_advanced_scheduler_settings_batch_load_failed", "error", err)
				fallbackValues := make(map[string]string)
				for _, key := range openAIAdvancedSchedulerRuntimeSettingKeys() {
					if value, valueErr := repo.GetValue(dbCtx, key); valueErr == nil {
						fallbackValues[key] = value
					}
				}
				enabled = strings.EqualFold(strings.TrimSpace(fallbackValues[openAIAdvancedSchedulerSettingKey]), "true")
				stickyWeightedEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled]), "true")
				subscriptionPriorityEnabled = strings.EqualFold(strings.TrimSpace(fallbackValues[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled]), "true")
				lbTopKOverride = parsePositiveIntOverride(fallbackValues[SettingKeyOpenAIAdvancedSchedulerLBTopK])
				weightOverrides = parseOpenAIAdvancedSchedulerWeightOverrides(fallbackValues)
			}
		}

		if settingSvc != nil {
			settingSvc.openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
				enabled:                     enabled,
				stickyWeightedEnabled:       stickyWeightedEnabled,
				subscriptionPriorityEnabled: subscriptionPriorityEnabled,
				lbTopKOverride:              lbTopKOverride,
				weightOverrides:             cloneOpenAIAdvancedSchedulerWeightOverrides(weightOverrides),
				expiresAt:                   time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
			})
		}
		return openAIAdvancedSchedulerRuntimeSettings{
			enabled:                     enabled,
			stickyWeightedEnabled:       stickyWeightedEnabled,
			subscriptionPriorityEnabled: subscriptionPriorityEnabled,
			lbTopKOverride:              lbTopKOverride,
			weightOverrides:             weightOverrides,
		}, nil
	}

	var result any
	if settingSvc != nil {
		result, _, _ = settingSvc.openAIAdvancedSchedulerSettingSF.Do(openAIAdvancedSchedulerSettingKey, doLoad)
	} else {
		result, _ = doLoad()
	}

	settings, _ := result.(openAIAdvancedSchedulerRuntimeSettings)
	return settings
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerEnabled(ctx context.Context) bool {
	return s.openAIAdvancedSchedulerRuntimeSettings(ctx).enabled
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx context.Context) bool {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.enabled && settings.stickyWeightedEnabled
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerSubscriptionPriorityEnabled(ctx context.Context) bool {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.enabled && settings.subscriptionPriorityEnabled
}

func openAIAdvancedSchedulerRuntimeSettingKeys() []string {
	keys := []string{
		openAIAdvancedSchedulerSettingKey,
		SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled,
		SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		SettingKeyOpenAIAdvancedSchedulerLBTopK,
	}
	for _, spec := range openAIAdvancedSchedulerWeightOverrideSpecs() {
		keys = append(keys, spec.key)
	}
	return keys
}

type openAIAdvancedSchedulerWeightOverrideSpec struct {
	key  string
	name string
}

func openAIAdvancedSchedulerWeightOverrideSpecs() []openAIAdvancedSchedulerWeightOverrideSpec {
	return []openAIAdvancedSchedulerWeightOverrideSpec{
		{key: SettingKeyOpenAIAdvancedSchedulerWeightPriority, name: "priority"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightLoad, name: "load"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightQueue, name: "queue"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightErrorRate, name: "error_rate"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightTTFT, name: "ttft"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightReset, name: "reset"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom, name: "quota_headroom"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse, name: "previous_response"},
		{key: SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky, name: "session_sticky"},
	}
}

func parsePositiveIntOverride(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func parseOpenAIAdvancedSchedulerWeightOverrides(values map[string]string) map[string]float64 {
	overrides := map[string]float64{}
	for _, spec := range openAIAdvancedSchedulerWeightOverrideSpecs() {
		raw := strings.TrimSpace(values[spec.key])
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		overrides[spec.name] = value
	}
	return overrides
}

func cloneOpenAIAdvancedSchedulerWeightOverrides(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s *OpenAIGatewayService) getOpenAIAccountScheduler(ctx context.Context) OpenAIAccountScheduler {
	if s == nil {
		return nil
	}
	if !s.isOpenAIAdvancedSchedulerEnabled(ctx) {
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
		decision.Layer = openAIAccountScheduleLayerLoadBalance
		if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
			effectiveExcludedIDs := scheduling.CloneExcluded(excludedIDs)
			for {
				selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, platform, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact, requiredCapability)
				if err != nil {
					return nil, decision, normalizeOpenAILegacyCompatibilityError(err, requestedModel)
				}
				if selection == nil || selection.Account == nil {
					return selection, decision, nil
				}
				if accountSupportsOpenAICapabilities(selection.Account, requiredCapability, requiredImageCapability) {
					return selection, decision, nil
				}
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				if scheduling.IsExcluded(effectiveExcludedIDs, selection.Account.ID) {
					return nil, decision, ErrNoAvailableAccounts
				}
				effectiveExcludedIDs = scheduling.AddExcluded(effectiveExcludedIDs, selection.Account.ID)
			}
		}

		effectiveExcludedIDs := scheduling.CloneExcluded(excludedIDs)
		for {
			selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, platform, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact, requiredCapability)
			if err != nil {
				return nil, decision, normalizeOpenAILegacyCompatibilityError(err, requestedModel)
			}
			if selection == nil || selection.Account == nil {
				return selection, decision, nil
			}
			if s.isOpenAIAccountTransportCompatible(selection.Account, requiredTransport) &&
				accountSupportsOpenAICapabilities(selection.Account, requiredCapability, requiredImageCapability) {
				return selection, decision, nil
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if scheduling.IsExcluded(effectiveExcludedIDs, selection.Account.ID) {
				return nil, decision, ErrNoAvailableAccounts
			}
			effectiveExcludedIDs = scheduling.AddExcluded(effectiveExcludedIDs, selection.Account.ID)
		}
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
	stickyWeighted := s.isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx)
	subscriptionPriority := s.isOpenAIAdvancedSchedulerSubscriptionPriorityEnabled(ctx)
	stickyPreviousAccountID := int64(0)
	if stickyWeighted && previousResponseCanMove && strings.TrimSpace(previousResponseID) != "" && platform == PlatformOpenAI {
		stickyPreviousAccountID = s.ResolveAccountIDByPreviousResponseIDForScheduler(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	}
	requiredCredentialClass := OpenAICredentialClassUnspecified
	// When failover already chose a credential class via excluded set + first failed account is unavailable here,
	// keep Unspecified for first hop. If sticky account is known, pin class to that account.
	userPinAPIKeyID := openAIUserPinAPIKeyIDFromContext(ctx)
	userPinAccountID := s.getUserPinnedAccountID(ctx, groupID, userPinAPIKeyID)
	for _, id := range []int64{stickyAccountID, stickyPreviousAccountID, userPinAccountID} {
		if id <= 0 {
			continue
		}
		if acc, err := s.getSchedulableAccount(ctx, id); err == nil && acc != nil {
			if class := OpenAIAccountCredentialClass(acc); class != OpenAICredentialClassUnspecified {
				requiredCredentialClass = class
				break
			}
		}
	}

	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:                 groupID,
		Platform:                platform,
		SessionHash:             sessionHash,
		StickyAccountID:         stickyAccountID,
		StickyPreviousAccountID: stickyPreviousAccountID,
		StickyWeighted:          stickyWeighted,
		SubscriptionPriority:    subscriptionPriority,
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

func normalizeOpenAILegacyCompatibilityError(err error, requestedModel string) error {
	if errors.Is(err, ErrNoAvailableAccounts) {
		return noAvailableOpenAISelectionError(requestedModel, false)
	}
	return err
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

func (s *OpenAIGatewayService) openAIWSLBTopK() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return s.cfg.Gateway.OpenAIWS.LBTopK
	}
	return 7
}

func (s *OpenAIGatewayService) openAIWSLBTopKForRequest(ctx context.Context) int {
	base := s.openAIWSLBTopK()
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	// DB 覆盖值与 stickyWeighted/subscriptionPriority 一样受总开关门控：
	// 关闭高级调度器后所有调用方（含管理页分数快照）都应回到配置/默认行为。
	if !settings.enabled {
		return base
	}
	if settings.lbTopKOverride > 0 {
		return settings.lbTopKOverride
	}
	return base
}

func (s *OpenAIGatewayService) openAIStickyEscapeConfig() openAIStickyEscapeConfig {
	if s != nil && s.cfg != nil {
		cfg := s.cfg.Gateway.OpenAIScheduler
		enabled := cfg.StickyEscapeEnabled
		if !enabled && cfg.StickyEscapeTTFTMs == 0 && cfg.StickyEscapeErrorRate == 0 {
			enabled = true
		}
		ttftMs := float64(cfg.StickyEscapeTTFTMs)
		if ttftMs <= 0 {
			ttftMs = 15000
		}
		errorRate := cfg.StickyEscapeErrorRate
		if errorRate < 0 || errorRate > 1 {
			errorRate = 0.5
		}
		if errorRate == 0 && cfg.StickyEscapeTTFTMs == 0 && cfg.StickyEscapeErrorRate == 0 {
			errorRate = 0.5
		}
		return openAIStickyEscapeConfig{
			enabled:   enabled,
			ttftMs:    ttftMs,
			errorRate: errorRate,
		}
	}
	return openAIStickyEscapeConfig{
		enabled:   true,
		ttftMs:    15000,
		errorRate: 0.5,
	}
}

func (s *OpenAIGatewayService) openAIWSSchedulerWeights() GatewayOpenAIWSSchedulerScoreWeightsView {
	defaults := GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:       1.0,
		Load:           1.0,
		Queue:          0.7,
		ErrorRate:      0.8,
		TTFT:           0.5,
		Reset:          0.0,
		QuotaHeadroom:  0.0,
		CompactError:   1.0,
		CompactLatency: 0.5,
		Previous:       5.0,
		SessionSticky:  3.0,
	}
	if s == nil || s.cfg == nil {
		return defaults
	}
	w := s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights
	view := GatewayOpenAIWSSchedulerScoreWeightsView{
		Priority:       w.Priority,
		Load:           w.Load,
		Queue:          w.Queue,
		ErrorRate:      w.ErrorRate,
		TTFT:           w.TTFT,
		Reset:          w.Reset,
		QuotaHeadroom:  w.QuotaHeadroom,
		CompactError:   w.CompactError,
		CompactLatency: w.CompactLatency,
		Previous:       w.PreviousResponse,
		SessionSticky:  w.SessionSticky,
	}
	// Compact weights default to non-zero in the defaults struct above and in
	// config.LoadConfig (guarded by viper.IsSet). Do NOT override explicit zero
	// here: zero is a valid way to disable compact penalties from config.
	return view
}

func (s *OpenAIGatewayService) openAIWSSchedulerWeightsForRequest(ctx context.Context) GatewayOpenAIWSSchedulerScoreWeightsView {
	weights := s.openAIWSSchedulerWeights()
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	// 同 openAIWSLBTopKForRequest：总开关关闭时不应用 DB 覆盖值。
	if !settings.enabled {
		return weights
	}
	return applyOpenAIAdvancedSchedulerWeightOverrides(weights, settings.weightOverrides)
}

func applyOpenAIAdvancedSchedulerWeightOverrides(
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	overrides map[string]float64,
) GatewayOpenAIWSSchedulerScoreWeightsView {
	for key, value := range overrides {
		switch key {
		case "priority":
			weights.Priority = value
		case "load":
			weights.Load = value
		case "queue":
			weights.Queue = value
		case "error_rate":
			weights.ErrorRate = value
		case "ttft":
			weights.TTFT = value
		case "reset":
			weights.Reset = value
		case "quota_headroom":
			weights.QuotaHeadroom = value
		case "compact_error":
			weights.CompactError = value
		case "compact_latency":
			weights.CompactLatency = value
		case "previous_response":
			weights.Previous = value
		case "session_sticky":
			weights.SessionSticky = value
		}
	}
	return weights
}

type GatewayOpenAIWSSchedulerScoreWeightsView struct {
	Priority  float64
	Load      float64
	Queue     float64
	ErrorRate float64
	TTFT      float64
	// Reset 倾向「会话窗口最早重置」的账号；0 表示关闭（默认）。
	Reset          float64
	QuotaHeadroom  float64
	CompactError   float64
	CompactLatency float64
	Previous       float64
	SessionSticky  float64
}

type OpenAIAccountSchedulerScoreSnapshot struct {
	BaseScore             float64
	StickyScore           float64
	StickyScoreInfinity   bool
	StickyWeightedEnabled bool
}

func (s *RateLimitService) BuildOpenAIAccountSchedulerScoreSnapshot(
	ctx context.Context,
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	gateway := &OpenAIGatewayService{cfg: nil, rateLimitService: s}
	if s != nil {
		gateway.cfg = s.cfg
	}
	return buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, gateway.openAIWSSchedulerWeightsForRequest(ctx), gateway.isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx))
}

func BuildOpenAIAccountSchedulerScoreSnapshot(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	gateway := &OpenAIGatewayService{}
	return buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, gateway.openAIWSSchedulerWeights(), false)
}

func buildOpenAIAccountSchedulerScoreSnapshot(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
	weights GatewayOpenAIWSSchedulerScoreWeightsView,
	stickyWeightedEnabled bool,
) map[int64]OpenAIAccountSchedulerScoreSnapshot {
	if len(accounts) == 0 {
		return nil
	}
	candidates := make([]openAIAccountCandidateScore, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, openAIAccountCandidateScore{
			account:   account,
			loadInfo:  loadInfo,
			errorRate: 0,
			ttft:      0,
			hasTTFT:   false,
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	minPriority, maxPriority := openAIAccountSchedulingPriority(candidates[0].account), openAIAccountSchedulingPriority(candidates[0].account)
	maxWaiting := 1
	for i := range candidates {
		candidate := &candidates[i]
		candidate.priority = openAIAccountSchedulingPriority(candidate.account)
		if candidate.priority < minPriority {
			minPriority = candidate.priority
		}
		if candidate.priority > maxPriority {
			maxPriority = candidate.priority
		}
		if candidate.loadInfo.WaitingCount > maxWaiting {
			maxWaiting = candidate.loadInfo.WaitingCount
		}
	}

	minResetRemaining, maxResetRemaining := 0.0, 0.0
	hasResetSample := false
	now := time.Now()
	if weights.Reset > 0 {
		for _, candidate := range candidates {
			end := candidate.account.SessionWindowEnd
			if end == nil || !now.Before(*end) {
				continue
			}
			remaining := end.Sub(now).Seconds()
			if !hasResetSample {
				minResetRemaining, maxResetRemaining = remaining, remaining
				hasResetSample = true
				continue
			}
			if remaining < minResetRemaining {
				minResetRemaining = remaining
			}
			if remaining > maxResetRemaining {
				maxResetRemaining = remaining
			}
		}
	}

	result := make(map[int64]OpenAIAccountSchedulerScoreSnapshot, len(candidates))
	for _, candidate := range candidates {
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(candidate.priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := 1 - clamp01(float64(candidate.loadInfo.LoadRate)/100.0)
		queueFactor := 1 - clamp01(float64(candidate.loadInfo.WaitingCount)/float64(maxWaiting))
		errorFactor := 1.0
		ttftFactor := 0.5
		resetFactor := 0.0
		if weights.Reset > 0 && hasResetSample {
			if end := candidate.account.SessionWindowEnd; end != nil && now.Before(*end) {
				if maxResetRemaining > minResetRemaining {
					resetFactor = 1 - clamp01((end.Sub(now).Seconds()-minResetRemaining)/(maxResetRemaining-minResetRemaining))
				} else {
					resetFactor = 1
				}
			}
		}
		quotaHeadroomFactor := 0.0
		if weights.QuotaHeadroom > 0 {
			quotaHeadroomFactor = openAIQuotaHeadroomFactor(candidate.account, now)
		}
		baseScore := weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor +
			weights.Reset*resetFactor +
			weights.QuotaHeadroom*quotaHeadroomFactor
		score := OpenAIAccountSchedulerScoreSnapshot{
			BaseScore:             baseScore,
			StickyWeightedEnabled: stickyWeightedEnabled,
			StickyScoreInfinity:   !stickyWeightedEnabled,
		}
		if stickyWeightedEnabled {
			score.StickyScore = baseScore + weights.Previous + weights.SessionSticky
		}
		result[candidate.account.ID] = score
	}
	return result
}

func openAIQuotaHeadroomFactor(account *Account, now time.Time) float64 {
	if account != nil && account.Platform == PlatformGrok {
		return grokQuotaHeadroomFactor(account, now)
	}
	if account == nil || len(account.Extra) == 0 || openAIQuotaHeadroomSnapshotStale(account.Extra, now) {
		return openAIQuotaHeadroomNeutralFactor
	}
	primaryUsedPercent, ok := resolveAccountExtraNumber(account.Extra, "codex_primary_used_percent", "codex_7d_used_percent")
	if !ok || openAIQuotaWindowResetAny(account.Extra, now, "primary", "7d") {
		return openAIQuotaHeadroomNeutralFactor
	}
	primaryUsedPercent = roundUsagePercent2(primaryUsedPercent)

	factor := 1 - clamp01(primaryUsedPercent/100)
	if secondaryUsedPercent, ok := resolveAccountExtraNumber(account.Extra, "codex_secondary_used_percent", "codex_5h_used_percent"); ok &&
		!openAIQuotaWindowResetAny(account.Extra, now, "secondary", "5h") {
		secondaryUsedPercent = roundUsagePercent2(secondaryUsedPercent)
		secondaryRemaining := 1 - clamp01(secondaryUsedPercent/100)
		if secondaryRemaining < openAIQuotaHeadroomSecondaryLowRemain {
			factor *= openAIQuotaHeadroomNeutralFactor
		}
	}
	return factor
}

func grokQuotaHeadroomFactor(account *Account, now time.Time) float64 {
	if account == nil || len(account.Extra) == 0 {
		return openAIQuotaHeadroomNeutralFactor
	}
	if billing, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billing != nil {
		if factor, ok := grokBillingHeadroomFactor(billing, now); ok {
			return factor
		}
	}
	if snapshot, err := grokQuotaSnapshotFromExtra(account.Extra); err == nil && snapshot != nil {
		if factor, ok := grokAPIQuotaHeadroomFactor(snapshot, now); ok {
			return factor
		}
	}
	return openAIQuotaHeadroomNeutralFactor
}

func grokBillingHeadroomFactor(snapshot *xai.BillingSnapshot, now time.Time) (float64, bool) {
	if snapshot == nil || snapshot.StatusCode < 200 || snapshot.StatusCode >= 300 {
		return 0, false
	}
	fetchedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(snapshot.FetchedAt))
	if err != nil || fetchedAt.After(now) || now.Sub(fetchedAt) >= grokQuotaHeadroomSnapshotStaleAfter {
		return 0, false
	}
	if snapshot.UsagePercent != nil {
		return 1 - clamp01(roundUsagePercent2(*snapshot.UsagePercent)/100), true
	}
	if snapshot.UsedPercent != nil {
		return 1 - clamp01(roundUsagePercent2(*snapshot.UsedPercent)/100), true
	}
	if snapshot.MonthlyLimitCents != nil && *snapshot.MonthlyLimitCents > 0 && snapshot.UsedCents != nil {
		return 1 - clamp01(float64(*snapshot.UsedCents)/float64(*snapshot.MonthlyLimitCents)), true
	}
	return 0, false
}

func grokAPIQuotaHeadroomFactor(snapshot *xai.QuotaSnapshot, now time.Time) (float64, bool) {
	if snapshot == nil {
		return 0, false
	}
	updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(snapshot.UpdatedAt))
	if err != nil || updatedAt.After(now) || now.Sub(updatedAt) >= grokQuotaHeadroomSnapshotStaleAfter {
		return 0, false
	}

	factor := 1.0
	found := false
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.Tokens} {
		if window == nil || window.Limit == nil || *window.Limit <= 0 || window.Remaining == nil {
			continue
		}
		resetAt, ok := grokQuotaWindowResetAt(window)
		if !ok || !now.Before(resetAt) {
			continue
		}
		remaining := clamp01(float64(*window.Remaining) / float64(*window.Limit))
		if !found || remaining < factor {
			factor = remaining
		}
		found = true
	}
	return factor, found
}

func grokQuotaWindowResetAt(window *xai.QuotaWindow) (time.Time, bool) {
	if window == nil {
		return time.Time{}, false
	}
	if window.ResetUnix != nil {
		return time.Unix(*window.ResetUnix, 0), true
	}
	resetAt, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt))
	return resetAt, err == nil
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
