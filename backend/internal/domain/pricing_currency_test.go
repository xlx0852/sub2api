package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePricingCurrencyDefaultsLegacyValuesToUSD(t *testing.T) {
	require.Equal(t, PricingCurrencyUSD, NormalizePricingCurrency(""))
	require.Equal(t, PricingCurrencyUSD, NormalizePricingCurrency("unknown"))
	require.Equal(t, PricingCurrencyUSD, NormalizePricingCurrency(" usd "))
	require.Equal(t, PricingCurrencyCNY, NormalizePricingCurrency(" cny "))
}

func TestIsSupportedPricingCurrencyRejectsEmptyAndUnknownValues(t *testing.T) {
	require.True(t, IsSupportedPricingCurrency("USD"))
	require.True(t, IsSupportedPricingCurrency("cny"))
	require.False(t, IsSupportedPricingCurrency(""))
	require.False(t, IsSupportedPricingCurrency("EUR"))
}
