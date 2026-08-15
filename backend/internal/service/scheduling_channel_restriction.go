package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

func checkChannelPricingRestriction(ctx context.Context, channelService *ChannelService, groupID *int64, requestedModel string) bool {
	if groupID == nil || channelService == nil || requestedModel == "" {
		return false
	}
	mapping := channelService.ResolveChannelMapping(ctx, *groupID, requestedModel)
	billingModel := scheduling.BillingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
	if billingModel == "" {
		return false
	}
	return channelService.IsModelRestricted(ctx, *groupID, billingModel)
}
