package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildPublicPricingSnapshotExcludesPrivateOffersAndChoosesLowestPublicPrice(t *testing.T) {
	baseInput := 0.00001
	channels := []service.AvailableChannel{
		{
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{
				{ID: 1, Platform: "openai", RateMultiplier: 0.8},
				{ID: 2, Platform: "openai", RateMultiplier: 0.5, IsExclusive: true},
			},
			SupportedModels: []service.SupportedModel{{
				Name: "gpt-public", Platform: "openai",
				Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &baseInput},
			}},
		},
		{
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{{ID: 3, Platform: "openai", RateMultiplier: 0.7}},
			SupportedModels: []service.SupportedModel{{
				Name: "gpt-public", Platform: "openai",
				Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &baseInput},
			}},
		},
		{
			Status: "disabled",
			Groups: []service.AvailableGroupRef{{ID: 4, Platform: "openai", RateMultiplier: 0.1}},
			SupportedModels: []service.SupportedModel{{
				Name: "gpt-public", Platform: "openai",
				Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeToken, InputPrice: &baseInput},
			}},
		},
	}

	snapshot := buildPublicPricingSnapshot(channels, nil, time.Unix(123, 0).UTC())
	require.Len(t, snapshot.Models, 1)
	require.Equal(t, "gpt-public", snapshot.Models[0].Name)
	require.NotNil(t, snapshot.Models[0].Pricing)
	require.NotNil(t, snapshot.Models[0].Pricing.InputPrice)
	require.InDelta(t, 0.000007, *snapshot.Models[0].Pricing.InputPrice, 0.0000000001)
}

func TestEncodePublicPricingSnapshotProducesStableETag(t *testing.T) {
	snapshot := &publicPricingSnapshot{Version: publicPricingCacheVersion, GeneratedAt: time.Unix(123, 0).UTC(), Models: []publicPricingModel{}}
	first, _, err := encodePublicPricingSnapshot(snapshot)
	require.NoError(t, err)
	second, _, err := encodePublicPricingSnapshot(snapshot)
	require.NoError(t, err)
	require.NotEmpty(t, first.etag)
	require.Equal(t, first.etag, second.etag)
}

func TestPublicPricingSnapshotCarriesCacheVersion(t *testing.T) {
	snapshot := buildPublicPricingSnapshot(nil, nil, time.Unix(123, 0).UTC())
	require.Equal(t, publicPricingCacheVersion, snapshot.Version)

	_, payload, err := encodePublicPricingSnapshot(snapshot)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(payload, &wire))
	require.EqualValues(t, publicPricingCacheVersion, wire["version"])
}

func TestSetPublicPricingCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	setPublicPricingCacheHeaders(ctx, `"etag"`)

	require.Equal(t, `"etag"`, recorder.Header().Get("ETag"))
	require.Contains(t, recorder.Header().Get("Cache-Control"), "s-maxage=3600")
	require.Contains(t, recorder.Header().Get("Cache-Control"), "stale-if-error=86400")
}
