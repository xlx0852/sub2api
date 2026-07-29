//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProfitHandler_GetOverviewCachesAndRefreshes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewProfitHandler(nil)
	var loads atomic.Int32
	h.loadOverview = func(context.Context, time.Time, time.Time, string) (*service.ProfitOverviewResponse, error) {
		load := loads.Add(1)
		return &service.ProfitOverviewResponse{
			GeneratedAt: time.Date(2026, 7, 29, 0, 0, int(load), 0, time.UTC),
			Summary:     &service.ProfitSummaryResponse{},
		}, nil
	}
	router := gin.New()
	router.GET("/overview", h.GetOverview)

	request := func(refresh bool) *httptest.ResponseRecorder {
		url := "/overview?start_date=2026-07-23&end_date=2026-07-29&timezone=UTC"
		if refresh {
			url += "&refresh=true"
		}
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		router.ServeHTTP(recorder, req)
		return recorder
	}

	first := request(false)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "miss", first.Header().Get("X-Profit-Snapshot-Cache"))
	require.NotEmpty(t, first.Header().Get("ETag"))

	second := request(false)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "hit", second.Header().Get("X-Profit-Snapshot-Cache"))
	require.Equal(t, first.Header().Get("ETag"), second.Header().Get("ETag"))
	require.Equal(t, int32(1), loads.Load())

	refreshed := request(true)
	require.Equal(t, http.StatusOK, refreshed.Code)
	require.Equal(t, "miss", refreshed.Header().Get("X-Profit-Snapshot-Cache"))
	require.NotEqual(t, second.Header().Get("ETag"), refreshed.Header().Get("ETag"))
	require.Equal(t, int32(2), loads.Load())

	h.invalidateOverviewCache()
	afterInvalidation := request(false)
	require.Equal(t, http.StatusOK, afterInvalidation.Code)
	require.Equal(t, "miss", afterInvalidation.Header().Get("X-Profit-Snapshot-Cache"))
	require.Equal(t, int32(3), loads.Load())
}

func TestProfitHandler_GetOverviewHonorsIfNoneMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewProfitHandler(nil)
	h.loadOverview = func(context.Context, time.Time, time.Time, string) (*service.ProfitOverviewResponse, error) {
		return &service.ProfitOverviewResponse{
			GeneratedAt: time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
			Summary:     &service.ProfitSummaryResponse{},
		}, nil
	}
	router := gin.New()
	router.GET("/overview", h.GetOverview)
	url := "/overview?start_date=2026-07-23&end_date=2026-07-29&timezone=UTC"

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, url, nil))
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	second := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	router.ServeHTTP(second, req)
	require.Equal(t, http.StatusNotModified, second.Code)
}

func TestProfitHandler_GetSupplyForecastCachesAndRefreshes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewProfitHandler(nil)
	var loads atomic.Int32
	h.loadSupplyForecast = func(context.Context, int, float64, string) (*service.SupplyForecastResponse, error) {
		load := loads.Add(1)
		return &service.SupplyForecastResponse{GeneratedAt: time.Date(2026, 7, 29, 0, 0, int(load), 0, time.UTC)}, nil
	}
	router := gin.New()
	router.GET("/supply-forecast", h.GetSupplyForecast)

	request := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/supply-forecast?timezone=UTC&horizon_days=30&safety_margin=0.2"+query, nil)
		router.ServeHTTP(recorder, req)
		return recorder
	}

	first := request("")
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "miss", first.Header().Get("X-Supply-Forecast-Cache"))
	second := request("")
	require.Equal(t, "hit", second.Header().Get("X-Supply-Forecast-Cache"))
	require.Equal(t, int32(1), loads.Load())
	refreshed := request("&refresh=true")
	require.Equal(t, "miss", refreshed.Header().Get("X-Supply-Forecast-Cache"))
	require.Equal(t, int32(2), loads.Load())

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/supply-forecast?horizon_days=31", nil))
	require.Equal(t, http.StatusBadRequest, invalid.Code)
}
