package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

// ErrOpenAIContinuationUnavailable is distinct from capacity exhaustion.
var ErrOpenAIContinuationUnavailable = errors.New("continuation_unavailable: previous response requires an unavailable account")

// One eligibility contract for preferences, candidate filtering and final
// rechecks. Slot ownership remains the responsibility of atomic acquire.
func (s *defaultOpenAIAccountScheduler) simpleCandidateDecision(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) scheduling.PolicyDecision {
	if s == nil || s.service == nil {
		return scheduling.PolicyDecision{RejectedPolicy: "scheduler_unavailable"}
	}
	r := openAICandidatePolicyRequest(req)
	r.IncludeFlatGroupIDs = true
	return scheduling.EvaluateCandidate(ctx, account, r,
		scheduling.NotExcludedPolicy, scheduling.PlatformPolicy, scheduling.GroupMembershipPolicy,
		scheduling.SchedulableForModelPolicy, openAICompatiblePolicy, providerQuotaPolicy,
		scheduling.NewCandidatePolicy("credential_class", func(_ context.Context, a *Account, _ scheduling.Request) bool {
			return OpenAICredentialClassMatches(a, req.RequiredCredentialClass)
		}),
		openAIRuntimePolicy{service: s.service}, openAIShadowParentPolicy{scheduler: s},
		openAIRequestedModelPolicy, openAIUpstreamChannelPolicy{service: s.service}, openAIAllCapabilitiesPolicy,
		openAITransportPolicy{service: s.service},
		scheduling.NewCandidatePolicy("privacy", func(_ context.Context, a *Account, _ scheduling.Request) bool {
			return req.simpleGroup == nil || !req.simpleGroup.RequirePrivacySet || a.IsPrivacySet()
		}),
	)
}

// Read the full account and then the request's DB prefetch (or DB directly).
// The same gates apply at both points, including group/class/runtime changes.
func (s *defaultOpenAIAccountScheduler) recheckSimpleCandidate(ctx context.Context, a *Account, req OpenAIAccountScheduleRequest) *Account {
	if a == nil {
		return nil
	}
	if s.service.schedulerSnapshot != nil {
		current, err := s.service.getSchedulableAccount(ctx, a.ID)
		if err != nil || current == nil {
			return nil
		}
		a = current
	}
	if !s.simpleCandidateDecision(ctx, a, req).Allowed {
		return nil
	}
	if s.service.accountRepo != nil {
		if prefetch := accountPrefetchFromContext(ctx); prefetch != nil {
			a = prefetch[a.ID]
		} else {
			current, err := s.service.accountRepo.GetByID(ctx, a.ID)
			if err != nil {
				return nil
			}
			a = current
		}
	}
	if !s.simpleCandidateDecision(ctx, a, req).Allowed {
		return nil
	}
	return a
}

func (s *defaultOpenAIAccountScheduler) prepareSimplePreferences(ctx context.Context, req *OpenAIAccountScheduleRequest) {
	valid := func(id int64, session bool) bool {
		if id <= 0 {
			return false
		}
		a, err := s.service.getSchedulableAccount(ctx, id)
		if err != nil || a == nil {
			return false
		}
		a = s.recheckSimpleCandidate(ctx, a, *req)
		return a != nil && (!session || s.service.stickySessionEpochMatches(ctx, req.GroupID, req.SessionHash, a))
	}
	if req.UserPinAccountID > 0 && !valid(req.UserPinAccountID, false) {
		slog.Info("openai_simple_affinity_invalid", "group_id", derefGroupID(req.GroupID), "source", "user_pin", "account_id", req.UserPinAccountID)
		s.service.deleteUserPin(ctx, req.GroupID, req.APIKeyID)
		req.UserPinAccountID = 0
	}
	if req.StickyAccountID > 0 && !valid(req.StickyAccountID, true) {
		slog.Info("openai_simple_affinity_invalid", "group_id", derefGroupID(req.GroupID), "source", "session", "account_id", req.StickyAccountID)
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
		req.StickyAccountID = 0
	}
}

func (s *defaultOpenAIAccountScheduler) buildSimpleLoadPlan(req OpenAIAccountScheduleRequest, accounts []*Account, loads map[int64]*AccountLoadInfo, seed uint64) openAIAccountLoadPlan {
	snapshots := make([]scheduling.CandidateSnapshot, 0, len(accounts))
	byID := make(map[int64]openAIAccountCandidate, len(accounts))
	for _, a := range accounts {
		c := scheduling.CandidateSnapshot{AccountID: a.ID, Priority: a.Priority}
		if load := loads[a.ID]; load != nil {
			c.Signals.LoadRate = scheduling.AvailableSignal(float64(load.LoadRate), time.Time{})
		}
		snapshots = append(snapshots, c)
		byID[a.ID] = openAIAccountCandidate{account: a, priority: a.Priority, loadInfo: loads[a.ID]}
	}
	rng := scheduling.NewSelectionRNG(seed)
	ordered := scheduling.OrderByPriorityAndLoad(snapshots, &rng)
	candidates := make([]openAIAccountCandidate, 0, len(ordered))
	for _, c := range ordered {
		candidates = append(candidates, byID[c.AccountID])
	}
	order := make([]openAIAccountCandidate, 0, len(candidates))
	preferred := map[int64]bool{}
	for _, id := range []int64{req.UserPinAccountID, req.StickyAccountID} {
		if c, ok := byID[id]; ok && !preferred[id] {
			order = append(order, c)
			preferred[id] = true
		}
	}
	for _, c := range candidates {
		if !preferred[c.account.ID] {
			order = append(order, c)
		}
	}
	// topK is the explored candidate count for the existing decision contract;
	// no eligible candidate is truncated in the simple profile.
	sum, sumSquares, n := 0.0, 0.0, 0
	for _, c := range candidates {
		if c.loadInfo != nil {
			v := float64(c.loadInfo.LoadRate)
			sum += v
			sumSquares += v * v
			n++
		}
	}
	return openAIAccountLoadPlan{allCandidates: candidates, candidates: candidates, selectionOrder: order, candidateCount: len(candidates), topK: len(candidates), loadSkew: calcLoadSkewByMoments(sum, sumSquares, n)}
}

func (s *defaultOpenAIAccountScheduler) selectSimplePreviousResponse(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, error) {
	_, a, responseID, store := s.service.resolveAccountByPreviousResponseIDForCapability(ctx, req.GroupID, req.PreviousResponseID, req.RequestedModel, req.ExcludedIDs, req.RequiredCapability, req.RequireCompact)
	a = s.recheckSimpleCandidate(ctx, a, req)
	if a == nil || (req.RequireCompact && openAICompactSupportTier(a) == 0) {
		if req.PreviousResponseCanMove {
			return nil, nil
		}
		return nil, ErrOpenAIContinuationUnavailable
	}
	result, err := s.service.tryAcquireAccountSlot(ctx, req.RequestedModel, a.ID, a.Concurrency)
	if err != nil {
		return nil, err
	}
	if result != nil && (result.Acquired || result.ModelLimited) {
		if result.Acquired {
			if store != nil {
				logOpenAIWSBindResponseAccountWarn(derefGroupID(req.GroupID), a.ID, responseID, store.BindResponseAccount(ctx, derefGroupID(req.GroupID), responseID, a.ID, s.service.openAIWSResponseStickyTTL()))
			}
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, a.ID)
			}
		}
		return &AccountSelectionResult{Account: a, Acquired: result.Acquired, ModelLimited: result.ModelLimited, ReleaseFunc: result.ReleaseFunc}, nil
	}
	if s.service.concurrencyService != nil {
		return &AccountSelectionResult{Account: a, WaitPlan: newStickyWaitPlan(a.ID, a.Concurrency, s.service.schedulingConfig())}, nil
	}
	return nil, ErrOpenAIContinuationUnavailable
}

func (s *defaultOpenAIAccountScheduler) recheckLoadCandidate(ctx context.Context, account *Account, req OpenAIAccountScheduleRequest) *Account {
	return s.recheckSimpleCandidate(ctx, account, req)
}
