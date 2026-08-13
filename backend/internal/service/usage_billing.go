package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

type UsageBillingCommand = domain.UsageBillingCommand
type UsageBillingRepository = ports.UsageBillingRepository

const UsageBillingMonetaryScale = domain.UsageBillingMonetaryScale

func QuantizeUsageBillingAmount(v float64) float64 {
	return domain.QuantizeUsageBillingAmount(v)
}

func HashUsageRequestPayload(payload []byte) string {
	return domain.HashUsageRequestPayload(payload)
}

func buildUsageBillingFingerprint(c *UsageBillingCommand) string {
	return domain.BuildUsageBillingFingerprint(c)
}
