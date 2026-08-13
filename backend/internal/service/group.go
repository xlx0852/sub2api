package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Group = domain.Group
type OpenAIMessagesDispatchModelConfig = domain.OpenAIMessagesDispatchModelConfig
type GroupModelsListConfig = domain.GroupModelsListConfig

func computePeakAwareMultipliers(apiKey *APIKey, base float64, now time.Time) (text, image float64) {
	return domain.ComputePeakAwareMultipliers(apiKey, base, now)
}
