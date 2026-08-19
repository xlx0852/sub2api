package billing

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BuildUsageLog constructs the canonical persisted usage row from a prepared
// provider-neutral input. Provider extractors must normalize media values and
// request identity before calling it.
func BuildUsageLog(input RecordInput, cost *CostBreakdown, effectiveRateMultiplier float64) *domain.UsageLog {
	durationMs := int(input.Duration.Milliseconds())
	pricingCurrency := string(domain.PricingCurrencyUSD)
	if cost != nil {
		pricingCurrency = string(domain.NormalizePricingCurrency(string(cost.PricingCurrency)))
	}
	usageLog := &domain.UsageLog{
		UserID:                input.User.ID,
		APIKeyID:              input.APIKey.ID,
		AccountID:             input.Account.ID,
		RequestID:             input.RequestID,
		Model:                 input.Model,
		RequestedModel:        input.RequestedModel,
		UpstreamModel:         optionalNonEqualString(input.UpstreamModel, input.Model),
		ServiceTier:           input.ServiceTier,
		ReasoningEffort:       input.ReasoningEffort,
		InboundEndpoint:       optionalString(input.InboundEndpoint),
		UpstreamEndpoint:      optionalString(input.UpstreamEndpoint),
		InputTokens:           input.Usage.InputTokens,
		OutputTokens:          input.Usage.OutputTokens,
		CacheCreationTokens:   input.Usage.CacheCreationTokens,
		CacheReadTokens:       input.Usage.CacheReadTokens,
		CacheCreation5mTokens: input.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: input.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     input.Usage.ImageOutputTokens,
		RateMultiplier:        effectiveRateMultiplier,
		PricingCurrency:       &pricingCurrency,
		AccountRateMultiplier: &input.AccountRateMultiplier,
		BillingType:           input.BillingType,
		BillingMode:           resolveBillingMode(input.ImageCount, cost),
		Stream:                input.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          input.FirstTokenMs,
		CacheTTLOverridden:    input.CacheTTLOverridden,
		ChannelID:             optionalInt64(input.ChannelID),
		ModelMappingChain:     optionalString(input.ModelMappingChain),
		UserAgent:             optionalString(input.UserAgent),
		IPAddress:             optionalString(input.IPAddress),
		GroupID:               input.APIKey.GroupID,
		ImageCount:            input.ImageCount,
		ImageSize:             optionalString(input.ImageSize),
		ImageInputSize:        optionalString(input.ImageInputSize),
		ImageOutputSize:       optionalString(input.ImageOutputSize),
		ImageSizeSource:       optionalString(input.ImageSizeSource),
		ImageSizeBreakdown:    input.ImageSizeBreakdown,
		OpenAIWSMode:          input.OpenAIWSMode,
		WSPayloadBytes:        input.WSPayloadBytes,
		WSEventCount:          input.WSEventCount,
		WSConnReused:          input.WSConnReused,
		WSConnPickMs:          input.WSConnPickMs,
		WSQueueWaitMs:         input.WSQueueWaitMs,
		CreatedAt:             time.Now(),
	}
	if input.Subscription != nil {
		usageLog.SubscriptionID = &input.Subscription.ID
	}
	if input.VideoUsage {
		usageLog.ImageSize = nil
		usageLog.VideoCount = input.VideoCount
		usageLog.VideoResolution = optionalString(input.VideoResolution)
		usageLog.VideoDurationSeconds = &input.VideoDurationSeconds
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
	}
	if input.Compact {
		usageLog.RequestType = domain.RequestTypeCompact
		usageLog.CompactPayloadBytes = input.CompactPayloadBytes
		usageLog.CompactRetryCount = &input.CompactRetryCount
		usageLog.CompactClientCanceled = &input.CompactClientCanceled
	}
	if input.CyberBlocked {
		usageLog.RequestType = domain.RequestTypeCyberBlocked
	}
	return usageLog
}

func resolveBillingMode(imageCount int, cost *CostBreakdown) *string {
	mode := "token"
	if cost != nil && cost.BillingMode != "" {
		mode = cost.BillingMode
	} else if imageCount > 0 {
		mode = "image"
	}
	return &mode
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalNonEqualString(value, compare string) *string {
	value = strings.TrimSpace(value)
	if value == "" || value == strings.TrimSpace(compare) {
		return nil
	}
	return &value
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
