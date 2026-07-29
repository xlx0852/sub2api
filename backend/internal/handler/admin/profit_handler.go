package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProfitHandler 账号利润分析
type ProfitHandler struct {
	profitService      *service.ProfitService
	overviewCache      *snapshotCache
	forecastCache      *snapshotCache
	loadOverview       func(context.Context, time.Time, time.Time, string) (*service.ProfitOverviewResponse, error)
	loadSupplyForecast func(context.Context, int, float64, string) (*service.SupplyForecastResponse, error)
}

// NewProfitHandler creates a new admin profit handler
func NewProfitHandler(profitService *service.ProfitService) *ProfitHandler {
	h := &ProfitHandler{
		profitService: profitService,
		overviewCache: newSnapshotCache(5 * time.Minute).WithMaxEntries(64),
		forecastCache: newSnapshotCache(15 * time.Minute).WithMaxEntries(32),
	}
	if profitService != nil {
		h.loadOverview = profitService.GetOverview
		h.loadSupplyForecast = profitService.GetSupplyForecast
	}
	return h
}

type supplyForecastCacheKey struct {
	HorizonDays  int     `json:"horizon_days"`
	SafetyMargin float64 `json:"safety_margin"`
	Timezone     string  `json:"timezone"`
}

type profitOverviewCacheKey struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

func (h *ProfitHandler) invalidateOverviewCache() {
	if h != nil && h.overviewCache != nil {
		h.overviewCache.Clear()
	}
}

// GetSummary 账号利润汇总
// GET /api/v1/admin/profit/summary?start_date=&end_date=&timezone=&account_id=
func (h *ProfitHandler) GetSummary(c *gin.Context) {
	start, end := parseTimeRange(c)
	var (
		summary *service.ProfitSummaryResponse
		err     error
	)
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		accountID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || accountID <= 0 {
			response.Error(c, 400, "Invalid account_id")
			return
		}
		summary, err = h.profitService.GetAccountSummary(c.Request.Context(), accountID, start, end)
	} else {
		summary, err = h.profitService.GetSummary(c.Request.Context(), start, end)
	}
	if err != nil {
		slog.Error("profit_summary_failed", "account_id", strings.TrimSpace(c.Query("account_id")), "error", err)
		response.Error(c, 500, "Failed to get profit summary")
		return
	}
	response.Success(c, summary)
}

// GetTrend 利润趋势
// GET /api/v1/admin/profit/trend?start_date=&end_date=&timezone=&account_id=
func (h *ProfitHandler) GetTrend(c *gin.Context) {
	start, end := parseTimeRange(c)
	var accountID *int64
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			accountID = &id
		}
	}
	userTZ := c.Query("timezone")
	tzName := timezone.Name()
	if userTZ != "" {
		if tz, err := time.LoadLocation(userTZ); err == nil {
			tzName = tz.String()
		}
	}
	trend, err := h.profitService.GetTrend(c.Request.Context(), accountID, start, end, tzName)
	if err != nil {
		slog.Error("profit_trend_failed", "account_id", strings.TrimSpace(c.Query("account_id")), "error", err)
		response.Error(c, 500, "Failed to get profit trend")
		return
	}
	response.Success(c, trend)
}

// GetOverview 利润页聚合数据。
// GET /api/v1/admin/profit/overview?start_date=&end_date=&timezone=
func (h *ProfitHandler) GetOverview(c *gin.Context) {
	start, end := parseTimeRange(c)
	tzName := timezone.Name()
	if userTZ := c.Query("timezone"); userTZ != "" {
		if tz, err := time.LoadLocation(userTZ); err == nil {
			tzName = tz.String()
		}
	}
	if h.loadOverview == nil {
		response.Error(c, 500, "Profit overview is unavailable")
		return
	}
	cacheKey := mustMarshalDashboardCacheKey(profitOverviewCacheKey{
		Start:    start.UTC().Format(time.RFC3339Nano),
		End:      end.UTC().Format(time.RFC3339Nano),
		Timezone: tzName,
	})
	load := func() (any, error) {
		return h.loadOverview(c.Request.Context(), start, end, tzName)
	}
	forceRefresh := parseBoolQueryWithDefault(c.Query("refresh"), false)
	var (
		cached snapshotCacheEntry
		hit    bool
		err    error
	)
	if forceRefresh {
		cached, err = h.overviewCache.Refresh(cacheKey, load)
	} else {
		cached, hit, err = h.overviewCache.GetOrLoad(cacheKey, load)
	}
	if err != nil {
		slog.Error("profit_overview_failed", "error", err)
		response.Error(c, 500, "Failed to get profit overview")
		return
	}
	c.Header("X-Profit-Snapshot-Cache", cacheStatusValue(hit))
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	response.Success(c, cached.Payload)
}

// GetSupplyForecast returns a cached operational estimate of stored-value demand and upstream supply.
// GET /api/v1/admin/profit/supply-forecast?horizon_days=30&safety_margin=0.2&timezone=
func (h *ProfitHandler) GetSupplyForecast(c *gin.Context) {
	if h.loadSupplyForecast == nil {
		response.Error(c, 500, "Supply forecast is unavailable")
		return
	}
	horizonDays := service.SupplyForecastDefaultHorizonDays
	if raw := strings.TrimSpace(c.Query("horizon_days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 7 && parsed != 30 && parsed != 60 && parsed != 90) {
			response.Error(c, 400, "horizon_days must be one of 7, 30, 60, 90")
			return
		}
		horizonDays = parsed
	}
	safetyMargin := service.SupplyForecastDefaultSafetyMargin
	if raw := strings.TrimSpace(c.Query("safety_margin")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			response.Error(c, 400, "safety_margin must be between 0 and 1")
			return
		}
		safetyMargin = parsed
	}
	tzName := timezone.Name()
	if userTZ := c.Query("timezone"); userTZ != "" {
		tz, err := time.LoadLocation(userTZ)
		if err != nil {
			response.Error(c, 400, "Invalid timezone")
			return
		}
		tzName = tz.String()
	}
	cacheKey := mustMarshalDashboardCacheKey(supplyForecastCacheKey{
		HorizonDays:  horizonDays,
		SafetyMargin: safetyMargin,
		Timezone:     tzName,
	})
	load := func() (any, error) {
		return h.loadSupplyForecast(c.Request.Context(), horizonDays, safetyMargin, tzName)
	}
	forceRefresh := parseBoolQueryWithDefault(c.Query("refresh"), false)
	var (
		cached snapshotCacheEntry
		hit    bool
		err    error
	)
	if forceRefresh {
		cached, err = h.forecastCache.Refresh(cacheKey, load)
	} else {
		cached, hit, err = h.forecastCache.GetOrLoad(cacheKey, load)
	}
	if err != nil {
		slog.Error("supply_forecast_failed", "error", err)
		response.Error(c, 500, "Failed to get supply forecast")
		return
	}
	c.Header("X-Supply-Forecast-Cache", cacheStatusValue(hit))
	if cached.ETag != "" {
		c.Header("ETag", cached.ETag)
		c.Header("Vary", "If-None-Match")
		if ifNoneMatchMatched(c.GetHeader("If-None-Match"), cached.ETag) {
			c.Status(http.StatusNotModified)
			return
		}
	}
	response.Success(c, cached.Payload)
}

// ListCostConfigs 成本配置列表
// GET /api/v1/admin/profit/configs
func (h *ProfitHandler) ListCostConfigs(c *gin.Context) {
	configs, err := h.profitService.ListCostConfigs(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "Failed to list cost configs")
		return
	}
	response.Success(c, configs)
}

type createSubscriptionCycleRequest struct {
	StartsAt   string  `json:"starts_at"`
	PeriodFee  float64 `json:"period_fee"`
	PeriodDays int     `json:"period_days"`
	Currency   string  `json:"currency"`
	Notes      string  `json:"notes"`
}

func (h *ProfitHandler) ListSubscriptionCycles(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.Error(c, 400, "Invalid account_id")
		return
	}
	result, err := h.profitService.ListSubscriptionCycles(c.Request.Context(), accountID)
	if err != nil {
		response.Error(c, 500, "Failed to list subscription cycles")
		return
	}
	response.Success(c, result)
}

func (h *ProfitHandler) CreateSubscriptionCycle(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.Error(c, 400, "Invalid account_id")
		return
	}
	var req createSubscriptionCycleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "Invalid request body")
		return
	}
	startsAt, err := time.Parse(time.DateOnly, req.StartsAt)
	if err != nil {
		response.Error(c, 400, "starts_at must be YYYY-MM-DD")
		return
	}
	cycle, err := h.profitService.CreateSubscriptionCycle(c.Request.Context(), &service.AccountSubscriptionCycle{AccountID: accountID, StartsAt: startsAt, PeriodFee: req.PeriodFee, PeriodDays: req.PeriodDays, Currency: req.Currency, Notes: req.Notes})
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	h.invalidateOverviewCache()
	response.Success(c, cycle)
}

func (h *ProfitHandler) DeleteSubscriptionCycle(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, 400, "Invalid cycle id")
		return
	}
	if err := h.profitService.DeleteSubscriptionCycle(c.Request.Context(), id); err != nil {
		response.Error(c, 500, "Failed to delete subscription cycle")
		return
	}
	h.invalidateOverviewCache()
	response.Success(c, gin.H{"deleted": true})
}

type upsertCostConfigRequest struct {
	CostType              string   `json:"cost_type"`
	PeriodFee             float64  `json:"period_fee"`
	PeriodDays            int      `json:"period_days"`
	Currency              string   `json:"currency"`
	WindowBaselineRevenue *float64 `json:"window_baseline_revenue"`
	Notes                 string   `json:"notes"`
}

// UpsertCostConfig 绑定/更新账号成本配置
// PUT /api/v1/admin/profit/configs/:account_id
func (h *ProfitHandler) UpsertCostConfig(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.Error(c, 400, "Invalid account_id")
		return
	}
	var req upsertCostConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "Invalid request body")
		return
	}
	cfg, err := h.profitService.UpsertCostConfig(c.Request.Context(), &service.AccountCostConfig{
		AccountID:             accountID,
		CostType:              req.CostType,
		PeriodFee:             req.PeriodFee,
		PeriodDays:            req.PeriodDays,
		Currency:              req.Currency,
		WindowBaselineRevenue: req.WindowBaselineRevenue,
		Notes:                 req.Notes,
	})
	if err != nil {
		slog.Error("profit_cost_config_save_failed", "account_id", accountID, "error", err)
		response.Error(c, 500, "Failed to save cost config")
		return
	}
	h.invalidateOverviewCache()
	response.Success(c, cfg)
}

// DeleteCostConfig 删除账号成本配置
// DELETE /api/v1/admin/profit/configs/:account_id
func (h *ProfitHandler) DeleteCostConfig(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.Error(c, 400, "Invalid account_id")
		return
	}
	if err := h.profitService.DeleteCostConfig(c.Request.Context(), accountID); err != nil {
		response.Error(c, 500, "Failed to delete cost config")
		return
	}
	h.invalidateOverviewCache()
	response.Success(c, gin.H{"deleted": true})
}

type batchSubscriptionConfigRequest struct {
	PeriodFee  float64 `json:"period_fee"`
	PeriodDays int     `json:"period_days"`
	Currency   string  `json:"currency"`
}

// BatchUpsertSubscriptionConfigs 批量为未配置的订阅类（OAuth）账号绑定订阅费用
// POST /api/v1/admin/profit/configs/batch
func (h *ProfitHandler) BatchUpsertSubscriptionConfigs(c *gin.Context) {
	var req batchSubscriptionConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "Invalid request body")
		return
	}
	if req.PeriodFee <= 0 {
		response.Error(c, 400, "period_fee must be positive")
		return
	}
	result, err := h.profitService.BatchUpsertSubscriptionConfigs(c.Request.Context(), req.PeriodFee, req.PeriodDays, req.Currency)
	if err != nil {
		response.Error(c, 500, "Failed to batch save cost configs")
		return
	}
	h.invalidateOverviewCache()
	response.Success(c, result)
}
