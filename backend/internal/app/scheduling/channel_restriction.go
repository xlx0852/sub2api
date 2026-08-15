package scheduling

import "github.com/Wei-Shaw/sub2api/internal/domain"

const (
	BillingModelSourceRequested     = domain.BillingModelSourceRequested
	BillingModelSourceUpstream      = domain.BillingModelSourceUpstream
	BillingModelSourceChannelMapped = domain.BillingModelSourceChannelMapped
)

// BillingModelForRestriction returns the model used by the channel pricing
// restriction gate. Upstream-based pricing is checked per account later.
func BillingModelForRestriction(source, requestedModel, channelMappedModel string) string {
	switch source {
	case BillingModelSourceRequested:
		return requestedModel
	case BillingModelSourceUpstream:
		return ""
	case BillingModelSourceChannelMapped:
		return channelMappedModel
	default:
		return channelMappedModel
	}
}
