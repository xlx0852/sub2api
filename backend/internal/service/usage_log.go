package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UsageLog = domain.UsageLog
const (
	BillingTypeBalance      = domain.BillingTypeBalance
	BillingTypeSubscription = domain.BillingTypeSubscription
)
func ParseUsageRequestType(value string) (RequestType, error) { return domain.ParseUsageRequestType(value) }
func RequestTypeFromLegacy(stream bool, openAIWSMode bool) RequestType {
	return domain.RequestTypeFromLegacy(stream, openAIWSMode)
}
