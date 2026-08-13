package service

import (
	"reflect"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Account = domain.Account
type OpenAIEndpointCapability = domain.OpenAIEndpointCapability
type TempUnschedulableRule = domain.TempUnschedulableRule
type WindowCostSchedulability = domain.WindowCostSchedulability

const (
	OpenAIEndpointCapabilityChatCompletions = domain.OpenAIEndpointCapabilityChatCompletions
	OpenAIEndpointCapabilityEmbeddings      = domain.OpenAIEndpointCapabilityEmbeddings
	OpenAIEndpointCapabilityAlphaSearch     = domain.OpenAIEndpointCapabilityAlphaSearch
	OpenAIAuthModePersonalAccessToken       = domain.OpenAIAuthModePersonalAccessToken
	WebSearchModeDefault                    = domain.WebSearchModeDefault
	WebSearchModeEnabled                    = domain.WebSearchModeEnabled
	WebSearchModeDisabled                   = domain.WebSearchModeDisabled
	WindowCostSchedulable                   = domain.WindowCostSchedulable
	WindowCostStickyOnly                    = domain.WindowCostStickyOnly
	WindowCostNotSchedulable                = domain.WindowCostNotSchedulable
)

func IsGroupContextValid(group *Group) bool {
	return domain.IsGroupContextValid(group)
}

func normalizeAccountNotes(value *string) *string {
	return domain.NormalizeAccountNotes(value)
}

func parseExtraFloat64(value any) float64 {
	return domain.ParseExtraFloat64(value)
}

func parseExtraInt(value any) int {
	return domain.ParseExtraInt(value)
}

func ParseExtraInt(value any) int {
	return domain.ParseExtraInt(value)
}

func ValidateQuotaResetConfig(extra map[string]any) error {
	return domain.ValidateQuotaResetConfig(extra)
}

func ComputeQuotaResetAt(extra map[string]any) {
	domain.ComputeQuotaResetAt(extra)
}

func NormalizeFixedQuotaWindows(extra map[string]any) {
	domain.NormalizeFixedQuotaWindows(extra)
}

func NormalizePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) (bool, string, string, float64) {
	return domain.NormalizePeakRateConfig(subscriptionType, enabled, start, end, multiplier)
}

func ValidatePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) error {
	return domain.ValidatePeakRateConfig(subscriptionType, enabled, start, end, multiplier)
}

func matchWildcard(pattern, str string) bool {
	return domain.MatchWildcard(pattern, str)
}

func mapPtr(m map[string]any) uintptr {
	if m == nil {
		return 0
	}
	return reflect.ValueOf(m).Pointer()
}

func stringMappingFromRaw(raw any) map[string]string {
	switch mapping := raw.(type) {
	case map[string]any:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			if str, ok := value.(string); ok {
				result[key] = str
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case map[string]string:
		if len(mapping) == 0 {
			return nil
		}
		result := make(map[string]string, len(mapping))
		for key, value := range mapping {
			result[key] = value
		}
		return result
	default:
		return nil
	}
}
