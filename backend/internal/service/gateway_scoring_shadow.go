package service

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

type gatewayShadowScore struct {
	AccountID int64
	Score     float64
	Breakdown scheduling.ScoreBreakdown
}

type gatewayShadowRuntimeSignal struct {
	AccountID           int64
	LoadRate            float64
	LoadRateAvailable   bool
	QueueDepth          float64
	QueueDepthAvailable bool
	ErrorRate           float64
	ErrorRateAvailable  bool
	TTFTMillis          float64
	TTFTAvailable       bool
	QuotaHeadroom       float64
	QuotaAvailable      bool
}

type gatewayShadowComparison struct {
	CandidateIDs              []int64
	EligibleCandidateIDs      []int64
	PolicyTrace               []scheduling.CandidatePolicyTrace
	Scores                    []gatewayShadowScore
	RuntimeSignals            []gatewayShadowRuntimeSignal
	SharedRank                []int64
	SharedTopK                []int64
	SharedSelectionOrder      []int64
	LegacySelectionCandidates []int64
	SelectionDiff             bool
	HardGateViolation         bool
	AffinityEscape            bool
	Fallback                  bool
	FallbackReason            string
}

func (s *GatewayService) evaluateGatewayScoringShadow(
	ctx context.Context,
	platform string,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	stickyAccountID int64,
	preferOAuth bool,
	preferSoonestReset bool,
	rawAccounts []Account,
	policyDecision scheduling.CandidateSetDecision,
	loadMap map[int64]*AccountLoadInfo,
	loadAvailable bool,
) gatewayShadowComparison {
	comparison := s.buildGatewayScoringComparison(
		ctx,
		platform,
		groupID,
		sessionHash,
		requestedModel,
		stickyAccountID,
		preferOAuth,
		preferSoonestReset,
		rawAccounts,
		policyDecision,
		loadMap,
		loadAvailable,
	)
	s.logGatewayScoringShadow(platform, groupID, comparison)
	return comparison
}

func (s *GatewayService) buildGatewayScoringComparison(
	ctx context.Context,
	platform string,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	stickyAccountID int64,
	preferOAuth bool,
	preferSoonestReset bool,
	rawAccounts []Account,
	policyDecision scheduling.CandidateSetDecision,
	loadMap map[int64]*AccountLoadInfo,
	loadAvailable bool,
) gatewayShadowComparison {
	now := time.Now()
	snapshots := make([]scheduling.CandidateSnapshot, 0, len(policyDecision.AllowedAccounts))
	runtimeSignals := make([]gatewayShadowRuntimeSignal, 0, len(policyDecision.AllowedAccounts))
	legacyLoadMap := make(map[int64]*AccountLoadInfo, len(policyDecision.AllowedAccounts))
	legacyLoadAvailable := loadAvailable || len(policyDecision.AllowedAccounts) > 0
	runtimeStatsWarmed := len(policyDecision.AllowedAccounts) > 0
	for _, account := range policyDecision.AllowedAccounts {
		if account == nil {
			continue
		}
		stats := s.gatewayRuntimeStatsSnapshot(account.ID, now)
		if loadInfo := loadMap[account.ID]; loadInfo != nil {
			stats.LoadRate = scheduling.AvailableSignal(float64(loadInfo.LoadRate), now)
			stats.QueueDepth = scheduling.AvailableSignal(float64(loadInfo.WaitingCount), now)
		}
		if stats.LoadRate.Available {
			legacyLoadMap[account.ID] = &AccountLoadInfo{
				AccountID:    account.ID,
				LoadRate:     int(stats.LoadRate.Value),
				WaitingCount: int(stats.QueueDepth.Value),
			}
		} else {
			legacyLoadAvailable = false
		}
		quotaHeadroom := strictestGatewayHeadroom(stats.QuotaHeadroom, stats.WindowHeadroom)
		if !stats.ErrorRate.Available || !stats.TTFTMillis.Available || !stats.LoadRate.Available {
			runtimeStatsWarmed = false
		}
		runtimeSignals = append(runtimeSignals, gatewayShadowRuntimeSignal{
			AccountID:           account.ID,
			LoadRate:            stats.LoadRate.Value,
			LoadRateAvailable:   stats.LoadRate.Available,
			QueueDepth:          stats.QueueDepth.Value,
			QueueDepthAvailable: stats.QueueDepth.Available,
			ErrorRate:           stats.ErrorRate.Value,
			ErrorRateAvailable:  stats.ErrorRate.Available,
			TTFTMillis:          stats.TTFTMillis.Value,
			TTFTAvailable:       stats.TTFTMillis.Available,
			QuotaHeadroom:       quotaHeadroom.Value,
			QuotaAvailable:      quotaHeadroom.Available,
		})
		resetSeconds := scheduling.UnavailableSignal()
		if account.SessionWindowEnd != nil && now.Before(*account.SessionWindowEnd) {
			resetSeconds = scheduling.AvailableSignal(account.SessionWindowEnd.Sub(now).Seconds(), now)
		}
		snapshots = append(snapshots, scheduling.NewCandidateSnapshot(scheduling.CandidateSnapshot{
			AccountID: account.ID,
			Platform:  account.Platform,
			Priority:  account.Priority,
			Signals: scheduling.CandidateSignals{
				LoadRate:       stats.LoadRate,
				QueueDepth:     stats.QueueDepth,
				ErrorRate:      stats.ErrorRate,
				TTFTMillis:     stats.TTFTMillis,
				QuotaHeadroom:  quotaHeadroom,
				WindowHeadroom: stats.WindowHeadroom,
				ResetSeconds:   resetSeconds,
			},
			Affinity: scheduling.AffinitySignals{SessionSticky: account.ID == stickyAccountID},
		}))
	}

	weights := s.gatewayShadowScoreWeights()
	scored := scheduling.ScoreCandidates(snapshots, weights, scheduling.ScoreOptions{StickyWeighted: true})
	ranked := append([]scheduling.ScoredCandidate(nil), scored...)
	scheduling.SortScoredCandidates(ranked)
	topK := scheduling.SelectTopK(scored, s.gatewayShadowTopK())
	seed := scheduling.DeriveSelectionSeed(scheduling.SelectionSeedInput{
		StickyKey:      sessionHash,
		RequestedModel: requestedModel,
		GroupID:        groupID,
		Entropy:        uint64(now.UnixNano()),
	})
	rng := scheduling.NewSelectionRNG(seed)
	selectionOrder := scheduling.BuildWeightedSelectionOrder(topK, &rng)
	legacySelectionCandidates := gatewayLegacySelectionCandidates(policyDecision.AllowedAccounts, legacyLoadMap, legacyLoadAvailable, preferOAuth, preferSoonestReset, now)
	sharedSelectionIDs := gatewayScoredCandidateIDs(selectionOrder)
	sharedSelectedID := firstInt64(sharedSelectionIDs)

	comparison := gatewayShadowComparison{
		CandidateIDs:              gatewayAccountValueIDs(rawAccounts),
		EligibleCandidateIDs:      openAIAccountPointerIDs(policyDecision.AllowedAccounts),
		PolicyTrace:               append([]scheduling.CandidatePolicyTrace(nil), policyDecision.Trace...),
		Scores:                    gatewayShadowScores(scored),
		RuntimeSignals:            runtimeSignals,
		SharedRank:                gatewayScoredCandidateIDs(ranked),
		SharedTopK:                gatewayScoredCandidateIDs(topK),
		SharedSelectionOrder:      sharedSelectionIDs,
		LegacySelectionCandidates: legacySelectionCandidates,
		SelectionDiff:             sharedSelectedID != 0 && !containsInt64(legacySelectionCandidates, sharedSelectedID),
		HardGateViolation:         hasOpenAIShadowSelectionOutsideEligible(sharedSelectionIDs, openAIAccountPointerIDs(policyDecision.AllowedAccounts)),
		AffinityEscape:            stickyAccountID > 0 && sharedSelectedID != stickyAccountID,
		Fallback:                  !runtimeStatsWarmed || sharedSelectedID == 0,
	}
	if !runtimeStatsWarmed {
		comparison.FallbackReason = "runtime_stats_not_warmed"
	} else if sharedSelectedID == 0 {
		comparison.FallbackReason = "no_counterfactual_selection"
	}
	return comparison
}

func (s *GatewayService) gatewayEnforceSelectionOrder(
	ctx context.Context,
	platform string,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	stickyAccountID int64,
	preferOAuth bool,
	preferSoonestReset bool,
	rawAccounts []Account,
	policyDecision scheduling.CandidateSetDecision,
	loadMap map[int64]*AccountLoadInfo,
) []int64 {
	comparison := s.buildGatewayScoringComparison(
		ctx,
		platform,
		groupID,
		sessionHash,
		requestedModel,
		stickyAccountID,
		preferOAuth,
		preferSoonestReset,
		rawAccounts,
		policyDecision,
		loadMap,
		true,
	)
	if comparison.HardGateViolation || comparison.Fallback || len(comparison.SharedSelectionOrder) == 0 {
		slog.Info("scheduler_scoring_enforce_fallback",
			"platform", platform,
			"group_id", derefGroupID(groupID),
			"hard_gate_violation", comparison.HardGateViolation,
			"fallback_reason", comparison.FallbackReason,
		)
		return nil
	}
	slog.Info("scheduler_scoring_enforce",
		"platform", platform,
		"group_id", derefGroupID(groupID),
		"eligible_candidate_ids", comparison.EligibleCandidateIDs,
		"selection_order", comparison.SharedSelectionOrder,
	)
	return comparison.SharedSelectionOrder
}

func orderGatewayAvailableByIDs(available []accountWithLoad, orderedIDs []int64) []accountWithLoad {
	if len(available) <= 1 || len(orderedIDs) == 0 {
		return available
	}
	byID := make(map[int64]accountWithLoad, len(available))
	for _, candidate := range available {
		if candidate.account != nil {
			byID[candidate.account.ID] = candidate
		}
	}
	ordered := make([]accountWithLoad, 0, len(available))
	seen := make(map[int64]struct{}, len(orderedIDs))
	for _, accountID := range orderedIDs {
		candidate, ok := byID[accountID]
		if !ok {
			continue
		}
		ordered = append(ordered, candidate)
		seen[accountID] = struct{}{}
	}
	for _, candidate := range available {
		if candidate.account == nil {
			continue
		}
		if _, ok := seen[candidate.account.ID]; !ok {
			ordered = append(ordered, candidate)
		}
	}
	return ordered
}

func (s *GatewayService) gatewayShadowScoreWeights() scheduling.ScoreWeights {
	weights := scheduling.ScoreWeights{
		Priority:         1,
		Load:             1,
		Queue:            0.7,
		ErrorRate:        0.8,
		TTFT:             0.5,
		PreviousResponse: 5,
		SessionSticky:    3,
	}
	if s == nil || s.cfg == nil {
		return weights
	}
	configured := s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights
	if configured.Priority+configured.Load+configured.Queue+configured.ErrorRate+configured.TTFT+configured.QuotaHeadroom <= 0 {
		return weights
	}
	return scheduling.ScoreWeights{
		Priority:         configured.Priority,
		Load:             configured.Load,
		Queue:            configured.Queue,
		ErrorRate:        configured.ErrorRate,
		TTFT:             configured.TTFT,
		Reset:            configured.Reset,
		QuotaHeadroom:    configured.QuotaHeadroom,
		PreviousResponse: configured.PreviousResponse,
		SessionSticky:    configured.SessionSticky,
	}
}

func (s *GatewayService) gatewayShadowTopK() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return s.cfg.Gateway.OpenAIWS.LBTopK
	}
	return 7
}

func strictestGatewayHeadroom(quota, window scheduling.Signal) scheduling.Signal {
	if !quota.Available {
		return window
	}
	if !window.Available || quota.Value <= window.Value {
		return quota
	}
	return window
}

func gatewayLegacySelectionCandidates(accounts []*Account, loadMap map[int64]*AccountLoadInfo, loadAvailable, preferOAuth, preferSoonestReset bool, now time.Time) []int64 {
	candidates := make([]scheduling.LoadCandidate, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		candidates = append(candidates, scheduling.LoadCandidate{Account: account, LoadRate: loadInfo.LoadRate})
	}
	var prefer func(*Account) bool
	if preferOAuth {
		prefer = func(account *Account) bool { return account.Type == AccountTypeOAuth }
	}
	best := scheduling.BestPriorityLoadLRUTies(candidates, scheduling.PriorityLoadOrderOptions{
		UseLoad:            loadAvailable,
		PreferSoonestReset: loadAvailable && preferSoonestReset,
		Prefer:             prefer,
		Now:                now,
	})
	result := make([]int64, 0, len(best))
	for _, candidate := range best {
		result = append(result, candidate.Account.ID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func gatewayShadowScores(candidates []scheduling.ScoredCandidate) []gatewayShadowScore {
	result := make([]gatewayShadowScore, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, gatewayShadowScore{AccountID: candidate.Candidate.AccountID, Score: candidate.Score(), Breakdown: candidate.Breakdown})
	}
	return result
}

func gatewayScoredCandidateIDs(candidates []scheduling.ScoredCandidate) []int64 {
	result := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.Candidate.AccountID)
	}
	return result
}

func gatewayAccountValueIDs(accounts []Account) []int64 {
	result := make([]int64, 0, len(accounts))
	for i := range accounts {
		result = append(result, accounts[i].ID)
	}
	return result
}

func candidatesFromAccountValues(accounts []Account) []*Account {
	result := make([]*Account, 0, len(accounts))
	for i := range accounts {
		result = append(result, &accounts[i])
	}
	return result
}

func firstInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func (s *GatewayService) logGatewayScoringShadow(platform string, groupID *int64, comparison gatewayShadowComparison) {
	counters := schedulerShadowCountersSnapshot{}
	if s != nil {
		counters = s.schedulerShadowMetrics.record(
			len(comparison.EligibleCandidateIDs),
			comparison.SelectionDiff,
			comparison.AffinityEscape,
			comparison.HardGateViolation,
			comparison.Fallback,
		)
	}
	slog.Info("scheduler_scoring_shadow",
		"platform", platform,
		"group_id", derefGroupID(groupID),
		"candidate_ids", comparison.CandidateIDs,
		"eligible_candidate_ids", comparison.EligibleCandidateIDs,
		"policy_trace", comparison.PolicyTrace,
		"shared_scores", comparison.Scores,
		"runtime_signals", comparison.RuntimeSignals,
		"shared_rank", comparison.SharedRank,
		"shared_top_k", comparison.SharedTopK,
		"shared_selection_order", comparison.SharedSelectionOrder,
		"legacy_selection_candidates", comparison.LegacySelectionCandidates,
		"selection_diff", comparison.SelectionDiff,
		"hard_gate_violation", comparison.HardGateViolation,
		"affinity_escape", comparison.AffinityEscape,
		"fallback", comparison.Fallback,
		"fallback_reason", comparison.FallbackReason,
		"evaluation_total", counters.EvaluationTotal,
		"candidate_total", counters.CandidateTotal,
		"selection_diff_total", counters.SelectionDiffTotal,
		"counterfactual_switch_total", counters.SelectionDiffTotal,
		"affinity_escape_total", counters.AffinityEscapeTotal,
		"hard_gate_violation_total", counters.HardGateViolationTotal,
		"fallback_total", counters.FallbackTotal,
	)
}
