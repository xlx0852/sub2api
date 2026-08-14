package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

type gatewayQuotaPolicy struct {
	service *GatewayService
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

func (s *GatewayService) isAccountAllowedForPlatform(account *Account, platform string, useMixed bool) bool {
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{Platform: platform, AllowMixedPlatform: useMixed},
		gatewayPlatformPolicy,
	).Allowed
}

func (s *GatewayService) isAccountSchedulableForQuota(account *Account) bool {
	return scheduling.EvaluateCandidate(
		context.Background(),
		account,
		scheduling.Request{},
		gatewayQuotaPolicy{service: s},
	).Allowed
}

func (s *GatewayService) isAccountSchedulableForWindowCost(ctx context.Context, account *Account, sticky bool) bool {
	return scheduling.EvaluateCandidate(
		ctx,
		account,
		scheduling.Request{Sticky: sticky},
		gatewayWindowCostPolicy{service: s},
	).Allowed
}

func (s *GatewayService) isAccountSchedulableForRPM(ctx context.Context, account *Account, sticky bool) bool {
	return scheduling.EvaluateCandidate(
		ctx,
		account,
		scheduling.Request{Sticky: sticky},
		gatewayRPMPolicy{service: s},
	).Allowed
}
