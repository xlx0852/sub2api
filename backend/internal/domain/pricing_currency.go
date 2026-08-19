package domain

import "strings"

// PricingCurrency describes the source denomination of model base prices.
// It is metadata only: billing never performs automatic FX conversion.
type PricingCurrency string

const (
	PricingCurrencyUSD PricingCurrency = "USD"
	PricingCurrencyCNY PricingCurrency = "CNY"
)

func NormalizePricingCurrency(value string) PricingCurrency {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(PricingCurrencyCNY):
		return PricingCurrencyCNY
	default:
		return PricingCurrencyUSD
	}
}

func IsSupportedPricingCurrency(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(PricingCurrencyUSD), string(PricingCurrencyCNY):
		return true
	default:
		return false
	}
}
