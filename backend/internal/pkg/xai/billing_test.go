//go:build unit

package xai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBillingResponseWeeklyAndProducts(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"config": {
			"creditUsagePercent": 10,
			"currentPeriod": {
				"type": "weekly",
				"start": "2026-07-05T14:38:00Z",
				"end": "2026-07-12T14:38:00Z"
			},
			"productUsage": [
				{"product": "GrokBuild", "usagePercent": 10},
				{"product": "Api", "usage_percent": null},
				{"product": "GrokChat", "usagePercent": 0}
			]
		}
	}`)
	snapshot, err := ParseBillingResponse(body)
	require.NoError(t, err)
	require.Equal(t, "weekly", snapshot.PeriodType)
	require.NotNil(t, snapshot.UsagePercent)
	require.InDelta(t, 10, *snapshot.UsagePercent, 0.001)
	require.Equal(t, "2026-07-05T14:38:00Z", snapshot.PeriodStart)
	require.Equal(t, "2026-07-12T14:38:00Z", snapshot.PeriodEnd)
	require.Len(t, snapshot.ProductUsage, 3)
	require.Equal(t, "GrokBuild", snapshot.ProductUsage[0].Product)
	require.NotNil(t, snapshot.ProductUsage[0].UsagePercent)
	require.InDelta(t, 10, *snapshot.ProductUsage[0].UsagePercent, 0.001)
	require.Nil(t, snapshot.ProductUsage[1].UsagePercent)
	require.NotNil(t, snapshot.ProductUsage[2].UsagePercent)
	require.InDelta(t, 0, *snapshot.ProductUsage[2].UsagePercent, 0.001)
}

func TestParseBillingResponseMonthlyCreditsObjectVal(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"config": {
			"monthlyLimit": {"val": 15000},
			"used": {"val": 7473},
			"onDemandCap": {"val": 0},
			"billingPeriodStart": "2026-07-01T00:00:00Z",
			"billingPeriodEnd": "2026-08-01T00:00:00Z"
		}
	}`)
	snapshot, err := ParseBillingResponse(body)
	require.NoError(t, err)
	require.Equal(t, PlanSuperGrok, snapshot.Plan)
	require.NotNil(t, snapshot.MonthlyLimitCents)
	require.EqualValues(t, 15000, *snapshot.MonthlyLimitCents)
	require.NotNil(t, snapshot.UsedCents)
	require.EqualValues(t, 7473, *snapshot.UsedCents)
	require.NotNil(t, snapshot.UsedPercent)
	require.InDelta(t, 49.82, *snapshot.UsedPercent, 0.1)
	require.Equal(t, "2026-08-01T00:00:00Z", snapshot.BillingPeriodEnd)
}

func TestParseBillingResponseProductsOnlyDoesNotForceWeekly(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"config": {
			"productUsage": [
				{"product": "GrokBuild", "usagePercent": 10},
				{"product": "Api"}
			]
		}
	}`)
	snapshot, err := ParseBillingResponse(body)
	require.NoError(t, err)
	require.Equal(t, "unknown", snapshot.PeriodType)
	require.Nil(t, snapshot.UsagePercent)
	require.Len(t, snapshot.ProductUsage, 2)
	require.Equal(t, "GrokBuild", snapshot.ProductUsage[0].Product)
}

func TestMergeBillingSnapshotsPrefersCreditsThenFillsMonthly(t *testing.T) {
	t.Parallel()

	weekly := &BillingSnapshot{
		PeriodType:   "weekly",
		UsagePercent: floatPtr(10),
		PeriodStart:  "2026-07-05T14:38:00Z",
		PeriodEnd:    "2026-07-12T14:38:00Z",
		ProductUsage: []BillingProductUsage{{Product: "GrokBuild", UsagePercent: floatPtr(10)}},
	}
	monthly := &BillingSnapshot{
		PeriodType:        "monthly",
		MonthlyLimitCents: int64Ptr(15000),
		UsedCents:         int64Ptr(7473),
		UsedPercent:       floatPtr(49.82),
		BillingPeriodEnd:  "2026-08-01T00:00:00Z",
		Plan:              PlanSuperGrok,
	}
	merged := MergeBillingSnapshots(weekly, monthly)
	require.Equal(t, "weekly", merged.PeriodType)
	require.InDelta(t, 10, *merged.UsagePercent, 0.001)
	require.Equal(t, "GrokBuild", merged.ProductUsage[0].Product)
	require.EqualValues(t, 15000, *merged.MonthlyLimitCents)
	require.Equal(t, PlanSuperGrok, merged.Plan)
}

func TestBuildBillingURL(t *testing.T) {
	t.Parallel()

	plain, err := BuildBillingURL(false)
	require.NoError(t, err)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing", plain)

	credits, err := BuildBillingURL(true)
	require.NoError(t, err)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", credits)
}

func TestApplyGrokCLIBillingHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "https://cli-chat-proxy.grok.com/v1/billing", nil)
	require.NoError(t, err)
	ApplyGrokCLIBillingHeaders(req, "tok", "user-1")
	require.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
	require.Equal(t, GrokCLITokenAuthValue, req.Header.Get(GrokCLITokenAuthHeader))
	require.Equal(t, "user-1", req.Header.Get(GrokCLIUserIDHeader))
}

func floatPtr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64     { return &v }
