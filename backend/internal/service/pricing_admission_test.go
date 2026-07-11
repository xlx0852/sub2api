//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingModelForAdmission(t *testing.T) {
	require.Equal(t, "requested", billingModelForAdmission(BillingModelSourceRequested, "requested", "mapped", "upstream"))
	require.Equal(t, "mapped", billingModelForAdmission(BillingModelSourceChannelMapped, "requested", "mapped", "upstream"))
	require.Equal(t, "upstream", billingModelForAdmission(BillingModelSourceUpstream, "requested", "mapped", "upstream"))
}

func TestValidatePricingAdmissionRejectsUnknownModel(t *testing.T) {
	resolver := NewModelPricingResolver(nil, newTestBillingServiceForResolver())
	require.ErrorIs(t, validatePricingAdmission(context.Background(), resolver, nil, "missing-model"), ErrPricingUnavailable)
	require.NoError(t, validatePricingAdmission(context.Background(), resolver, nil, "claude-sonnet-4"))
}
