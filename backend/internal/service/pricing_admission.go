package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrPricingUnavailable = infraerrors.ServiceUnavailable("PRICING_UNAVAILABLE", "Model pricing is unavailable. Please contact the administrator.")

func billingModelForAdmission(source, requestedModel, channelMappedModel, upstreamModel string) string {
	switch strings.TrimSpace(source) {
	case BillingModelSourceRequested:
		return strings.TrimSpace(requestedModel)
	case BillingModelSourceUpstream:
		return strings.TrimSpace(upstreamModel)
	default:
		if model := strings.TrimSpace(channelMappedModel); model != "" {
			return model
		}
		return strings.TrimSpace(requestedModel)
	}
}

func validatePricingAdmission(ctx context.Context, resolver *ModelPricingResolver, groupID *int64, model string) error {
	if resolver == nil || strings.TrimSpace(model) == "" {
		return nil
	}
	resolved := resolver.Resolve(ctx, PricingInput{Model: model, GroupID: groupID})
	if resolved == nil {
		return ErrPricingUnavailable
	}
	if resolved.Mode == BillingModeToken && resolved.BasePricing == nil {
		return ErrPricingUnavailable
	}
	if (resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) && resolved.Source != PricingSourceChannel {
		return ErrPricingUnavailable
	}
	return nil
}

func (s *GatewayService) ValidatePricingAdmission(ctx context.Context, groupID *int64, source, requestedModel, channelMappedModel, upstreamModel string) error {
	if s == nil {
		return nil
	}
	model := billingModelForAdmission(source, requestedModel, channelMappedModel, upstreamModel)
	return validatePricingAdmission(ctx, s.resolver, groupID, model)
}

func (s *OpenAIGatewayService) ValidatePricingAdmission(ctx context.Context, groupID *int64, source, requestedModel, channelMappedModel, upstreamModel string) error {
	if s == nil {
		return nil
	}
	model := billingModelForAdmission(source, requestedModel, channelMappedModel, upstreamModel)
	return validatePricingAdmission(ctx, s.resolver, groupID, model)
}
