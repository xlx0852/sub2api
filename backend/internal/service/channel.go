package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Channel = domain.Channel
type ChannelUsageFields = domain.ChannelUsageFields
type SupportedModel = domain.SupportedModel

const (
	BillingModelSourceRequested     = domain.BillingModelSourceRequested
	BillingModelSourceUpstream      = domain.BillingModelSourceUpstream
	BillingModelSourceChannelMapped = domain.BillingModelSourceChannelMapped
)

func splitWildcardSuffix(pattern string) (prefix string, isWildcard bool) {
	return domain.SplitWildcardSuffix(pattern)
}

func ValidateIntervals(intervals []PricingInterval, mode BillingMode) error {
	return domain.ValidateIntervals(intervals, mode)
}
