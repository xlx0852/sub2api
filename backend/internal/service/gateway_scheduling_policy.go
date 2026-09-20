package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

type gatewayQuotaPolicy struct {
	service *GatewayService
}

type gatewayModelCapabilityPolicy struct {
	service *GatewayService
}

type gatewayPrivacyPolicy struct {
	group *Group
}

type gatewayUpstreamChannelPolicy struct {
	service *GatewayService
	enabled bool
}

type gatewayCandidatePolicyOptions struct {
	group                *Group
	includeGroup         bool
	checkUpstreamChannel bool
}

var gatewayPlatformPolicy = scheduling.NewCandidatePolicy("gateway_platform", func(_ context.Context, account *Account, req scheduling.Request) bool {
	if account == nil {
		return false
	}
	if !req.AllowMixedPlatform {
		return account.Platform == req.Platform
	}
	return account.Platform == req.Platform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled())
})

func (p gatewayModelCapabilityPolicy) Name() string {
	return "model_capability"
}

func (p gatewayModelCapabilityPolicy) Allows(ctx context.Context, account *Account, req scheduling.Request) bool {
	return p.service != nil && account != nil &&
		(req.RequestedModel == "" || p.service.isModelSupportedByAccountWithContext(ctx, account, req.RequestedModel))
}

func (p gatewayPrivacyPolicy) Name() string {
	return "privacy"
}

func (p gatewayPrivacyPolicy) Allows(_ context.Context, account *Account, _ scheduling.Request) bool {
	return account != nil && (p.group == nil || !p.group.RequirePrivacySet || account.IsPrivacySet())
}

func (p gatewayUpstreamChannelPolicy) Name() string {
	return "upstream_channel"
}

func (p gatewayUpstreamChannelPolicy) Allows(ctx context.Context, account *Account, req scheduling.Request) bool {
	if !p.enabled {
		return true
	}
	return p.service != nil && account != nil && req.GroupID != nil &&
		!p.service.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel)
}

func (p gatewayQuotaPolicy) Name() string {
	return "quota"
}

func (p gatewayQuotaPolicy) Allows(_ context.Context, account *Account, _ scheduling.Request) bool {
	return p.service != nil && account != nil && p.service.accountPassesQuota(account)
}

type gatewayWindowCostPolicy struct {
	service *GatewayService
}

func (p gatewayWindowCostPolicy) Name() string {
	return "window_cost"
}

func (p gatewayWindowCostPolicy) Allows(ctx context.Context, account *Account, req scheduling.Request) bool {
	return p.service != nil && account != nil && p.service.accountPassesWindowCost(ctx, account, req.Sticky)
}

type gatewayRPMPolicy struct {
	service *GatewayService
}

func (p gatewayRPMPolicy) Name() string {
	return "rpm"
}

func (p gatewayRPMPolicy) Allows(ctx context.Context, account *Account, req scheduling.Request) bool {
	return p.service != nil && account != nil && p.service.accountPassesRPM(ctx, account, req.Sticky)
}

func (s *GatewayService) gatewayLimitDecision(ctx context.Context, account *Account, sticky bool) scheduling.PolicyDecision {
	return scheduling.EvaluateCandidate(
		ctx,
		account,
		scheduling.Request{Sticky: sticky},
		gatewayQuotaPolicy{service: s},
		gatewayWindowCostPolicy{service: s},
		gatewayRPMPolicy{service: s},
	)
}

func (s *GatewayService) gatewayCandidatePolicies() []scheduling.CandidatePolicy {
	return s.gatewayCandidatePoliciesFor(gatewayCandidatePolicyOptions{})
}

func (s *GatewayService) gatewayCandidatePoliciesFor(options gatewayCandidatePolicyOptions) []scheduling.CandidatePolicy {
	policies := []scheduling.CandidatePolicy{
		scheduling.NotExcludedPolicy,
		scheduling.SchedulablePolicy,
	}
	if options.includeGroup {
		policies = append(policies, scheduling.GroupMembershipPolicy)
	}
	if options.group != nil && options.group.RequirePrivacySet {
		policies = append(policies, gatewayPrivacyPolicy{group: options.group})
	}
	return append(policies,
		gatewayPlatformPolicy,
		gatewayModelCapabilityPolicy{service: s},
		gatewayUpstreamChannelPolicy{service: s, enabled: options.checkUpstreamChannel},
		scheduling.SchedulableForModelPolicy,
		gatewayQuotaPolicy{service: s},
		gatewayWindowCostPolicy{service: s},
		gatewayRPMPolicy{service: s},
	)
}

func (s *GatewayService) evaluateGatewayCandidates(
	ctx context.Context,
	accounts []*Account,
	request scheduling.Request,
) scheduling.CandidateSetDecision {
	return scheduling.EvaluateCandidates(ctx, accounts, request, s.gatewayCandidatePolicies()...)
}

func (s *GatewayService) isAccountAllowedForPlatform(account *Account, platform string, useMixed bool) bool {
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{Platform: platform, AllowMixedPlatform: useMixed},
		gatewayPlatformPolicy,
	).Allowed
}
