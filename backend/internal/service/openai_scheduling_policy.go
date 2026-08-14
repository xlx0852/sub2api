package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

type openAIRuntimePolicy struct {
	service *OpenAIGatewayService
}

type openAITransportPolicy struct {
	service *OpenAIGatewayService
}

func (p openAITransportPolicy) Name() string {
	return "transport"
}

func (p openAITransportPolicy) Allows(_ context.Context, account *Account, req scheduling.Request) bool {
	required := OpenAIUpstreamTransport(req.RequiredTransport)
	if required == OpenAIUpstreamTransportAny || required == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	return account != nil && p.service != nil && p.service.isOpenAIAccountTransportCompatible(account, required)
}

func (p openAIRuntimePolicy) Name() string {
	return "runtime_block"
}

func (p openAIRuntimePolicy) Allows(_ context.Context, account *Account, _ scheduling.Request) bool {
	return p.service != nil && account != nil && !p.service.isOpenAIAccountRuntimeBlocked(account)
}

var openAIAdvancedQuotaPolicy = scheduling.NewCandidatePolicy("advanced_quota_auto_pause", func(ctx context.Context, account *Account, _ scheduling.Request) bool {
	if account == nil {
		return false
	}
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account)
	return !paused
})

type openAIShadowParentPolicy struct {
	scheduler *defaultOpenAIAccountScheduler
}

func (p openAIShadowParentPolicy) Name() string {
	return "shadow_parent"
}

func (p openAIShadowParentPolicy) Allows(ctx context.Context, account *Account, _ scheduling.Request) bool {
	if p.scheduler == nil || account == nil {
		return false
	}
	return parentHealthyForShadow(account, func(id int64) *Account {
		return p.scheduler.lookupShadowParentAccount(ctx, id)
	})
}

type openAIUpstreamChannelPolicy struct {
	service *OpenAIGatewayService
}

func (p openAIUpstreamChannelPolicy) Name() string {
	return "upstream_channel"
}

func (p openAIUpstreamChannelPolicy) Allows(ctx context.Context, account *Account, req scheduling.Request) bool {
	if p.service == nil || account == nil {
		return false
	}
	if req.GroupID == nil || !p.service.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID) {
		return true
	}
	return !p.service.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact)
}

var openAIAllCapabilitiesPolicy = scheduling.NewCandidatePolicy("all_capabilities", func(_ context.Context, account *Account, req scheduling.Request) bool {
	return accountSupportsOpenAICapabilities(
		account,
		OpenAIEndpointCapability(req.RequiredCapability),
		OpenAIImagesCapability(req.RequiredImageCapability),
	)
})

func openAICandidatePolicyRequest(req OpenAIAccountScheduleRequest) scheduling.Request {
	return scheduling.Request{
		Platform:                normalizeOpenAICompatiblePlatform(req.Platform),
		RequestedModel:          req.RequestedModel,
		GroupID:                 req.GroupID,
		ExcludedIDs:             req.ExcludedIDs,
		RequireCompact:          req.RequireCompact,
		RequiredCapability:      string(req.RequiredCapability),
		RequiredImageCapability: string(req.RequiredImageCapability),
		RequiredTransport:       string(req.RequiredTransport),
	}
}
