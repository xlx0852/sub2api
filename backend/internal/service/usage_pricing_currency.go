package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

func NormalizeUsagePricingCurrency(value *string) domain.PricingCurrency {
	if value == nil {
		return domain.PricingCurrencyUSD
	}
	return domain.NormalizePricingCurrency(*value)
}
