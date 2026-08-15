package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type gatewaySchedulingStrategy struct{ service *GatewayService }

func (gatewaySchedulingStrategy) Name() string { return "gateway" }

func (gatewaySchedulingStrategy) Supports(req scheduling.Request) bool {
	switch req.Platform {
	case "", PlatformAnthropic, PlatformGemini, PlatformAntigravity:
		return true
	default:
		return false
	}
}

func (s gatewaySchedulingStrategy) Select(ctx context.Context, req scheduling.Request) (*scheduling.Outcome, error) {
	if req.ForcePlatform != "" {
		ctx = context.WithValue(ctx, ctxkey.ForcePlatform, req.ForcePlatform)
	} else if req.AllowMixedPlatform && req.Platform != "" {
		ctx = context.WithValue(ctx, gatewayPlatformHintContextKey{}, req.Platform)
	}
	service := s.service
	if req.SkipAcquire && service != nil {
		copyService := *service
		copyService.concurrencyService = nil
		service = &copyService
	}
	result, err := service.selectAccountWithLoadAwarenessStrategy(
		ctx, resolveSelectorGroupID(req), req.StickyKey, req.RequestedModel,
		req.ExcludedIDs, req.MetadataUserID, req.Sub2APIUserID,
	)
	if err != nil {
		return nil, err
	}
	return &scheduling.Outcome{Result: mapAccountSelectionResult(result)}, nil
}

type openAISchedulingStrategy struct{ service *OpenAIGatewayService }

func (openAISchedulingStrategy) Name() string { return "openai-compatible" }

func (openAISchedulingStrategy) Supports(req scheduling.Request) bool {
	switch req.Platform {
	case "", PlatformOpenAI, PlatformGrok, PlatformKimi:
		return true
	default:
		return false
	}
}

func (s openAISchedulingStrategy) Select(ctx context.Context, req scheduling.Request) (*scheduling.Outcome, error) {
	platform := req.Platform
	if platform == "" {
		platform = PlatformOpenAI
	}
	service := s.service
	if req.SkipAcquire && service != nil {
		copyService := *service
		copyService.concurrencyService = nil
		service = &copyService
	}
	result, decision, err := service.selectAccountWithSchedulerStrategy(
		ctx,
		resolveSelectorGroupID(req),
		req.PreviousResponseID,
		req.StickyKey,
		req.RequestedModel,
		req.ExcludedIDs,
		OpenAIUpstreamTransport(req.RequiredTransport),
		OpenAIEndpointCapability(req.RequiredCapability),
		OpenAIImagesCapability(req.RequiredImageCapability),
		req.RequireCompact,
		platform,
		req.PreviousResponseCanMove,
	)
	return &scheduling.Outcome{Result: mapAccountSelectionResult(result), Decision: decision}, err
}

// NewGatewayAccountSelector 创建 Gateway 平台策略的统一编排入口。
func NewGatewayAccountSelector(service *GatewayService) scheduling.AccountSelector {
	return scheduling.NewEngine(gatewaySchedulingStrategy{service: service})
}

// NewOpenAIAccountSelector 创建 OpenAI/Grok/Kimi 平台策略的统一编排入口。
func NewOpenAIAccountSelector(service *OpenAIGatewayService) *scheduling.Engine {
	return scheduling.NewEngine(openAISchedulingStrategy{service: service})
}

// NewUnifiedAccountSelector 创建可在全平台间分派的单一调度编排器。
func NewUnifiedAccountSelector(gateway *GatewayService, openAI *OpenAIGatewayService) scheduling.AccountSelector {
	strategies := make([]scheduling.Strategy, 0, 2)
	if gateway != nil {
		strategies = append(strategies, gatewaySchedulingStrategy{service: gateway})
	}
	if openAI != nil {
		strategies = append(strategies, openAISchedulingStrategy{service: openAI})
	}
	return scheduling.NewEngine(strategies...)
}

// SelectAccountWithLoadAwareness 保留原有公开入口，内部反向调用统一编排器。
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	selector := NewGatewayAccountSelector(s)
	return AccountSelectionFromScheduling(selector.Select(ctx, scheduling.Request{
		Platform:       PlatformAnthropic,
		GroupID:        groupID,
		StickyKey:      sessionHash,
		RequestedModel: requestedModel,
		ExcludedIDs:    excludedIDs,
		MetadataUserID: metadataUserID,
		Sub2APIUserID:  sub2apiUserID,
	}))
}

func (s *OpenAIGatewayService) selectAccountThroughEngine(
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
	engine := s.accountSelector
	if engine == nil {
		engine = NewOpenAIAccountSelector(s)
	}
	outcome, err := engine.SelectOutcome(ctx, scheduling.Request{
		Platform:                platform,
		GroupID:                 groupID,
		StickyKey:               sessionHash,
		PreviousResponseID:      previousResponseID,
		PreviousResponseCanMove: previousResponseCanMove,
		RequestedModel:          requestedModel,
		ExcludedIDs:             excludedIDs,
		RequiredTransport:       string(requiredTransport),
		RequiredCapability:      string(requiredCapability),
		RequiredImageCapability: string(requiredImageCapability),
		RequireCompact:          requireCompact,
	})
	decision := OpenAIAccountScheduleDecision{}
	if outcome != nil {
		decision, _ = outcome.Decision.(OpenAIAccountScheduleDecision)
	}
	if err != nil {
		return nil, decision, err
	}
	selection := accountSelectionFromSchedulingResult(outcome.Result)
	return selection, decision, nil
}

func resolveSelectorGroupID(req scheduling.Request) *int64 {
	if req.GroupID != nil {
		return req.GroupID
	}
	if req.Group != nil {
		id := req.Group.ID
		return &id
	}
	return nil
}

// AccountSelectionFromScheduling 把统一选号结果还原为现有 handler 使用的 AccountSelectionResult。
func AccountSelectionFromScheduling(sel *scheduling.Result, err error) (*AccountSelectionResult, error) {
	if err != nil {
		return nil, err
	}
	return accountSelectionFromSchedulingResult(sel), nil
}

func accountSelectionFromSchedulingResult(sel *scheduling.Result) *AccountSelectionResult {
	if sel == nil {
		return nil
	}
	out := &AccountSelectionResult{
		Account:      sel.Account,
		Acquired:     sel.Acquired,
		ReleaseFunc:  sel.Release,
		ModelLimited: sel.ModelLimited,
	}
	if sel.WaitPlan != nil {
		out.WaitPlan = &AccountWaitPlan{
			AccountID:      sel.WaitPlan.AccountID,
			MaxConcurrency: sel.WaitPlan.MaxConcurrency,
			Timeout:        sel.WaitPlan.Timeout,
			MaxWaiting:     sel.WaitPlan.MaxWaiting,
		}
	}
	return out
}

func mapAccountSelection(sel *AccountSelectionResult, err error) (*scheduling.Result, error) {
	if err != nil {
		return nil, err
	}
	return mapAccountSelectionResult(sel), nil
}

func mapAccountSelectionResult(sel *AccountSelectionResult) *scheduling.Result {
	if sel == nil {
		return nil
	}
	out := &scheduling.Result{
		Account:      sel.Account,
		Acquired:     sel.Acquired,
		Release:      sel.ReleaseFunc,
		ModelLimited: sel.ModelLimited,
	}
	if sel.WaitPlan != nil {
		out.WaitPlan = &scheduling.WaitPlan{
			AccountID:      sel.WaitPlan.AccountID,
			MaxConcurrency: sel.WaitPlan.MaxConcurrency,
			Timeout:        sel.WaitPlan.Timeout,
			MaxWaiting:     sel.WaitPlan.MaxWaiting,
		}
	}
	return out
}
