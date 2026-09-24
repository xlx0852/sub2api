package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetDashboardOverview returns vNext ops dashboard overview (raw path).
// GET /api/v1/admin/ops/dashboard/overview
func (h *OpsHandler) GetDashboardOverview(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.RequestType = requestType
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetDashboardOverview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardThroughputTrend returns throughput time series (raw path).
// GET /api/v1/admin/ops/dashboard/throughput-trend
func (h *OpsHandler) GetDashboardThroughputTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.RequestType = requestType
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetThroughputTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardLatencyHistogram returns the latency distribution histogram (success requests).
// GET /api/v1/admin/ops/dashboard/latency-histogram
func (h *OpsHandler) GetDashboardLatencyHistogram(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.RequestType = requestType
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetLatencyHistogram(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardErrorTrend returns error counts time series (raw path).
// GET /api/v1/admin/ops/dashboard/error-trend
func (h *OpsHandler) GetDashboardErrorTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.RequestType = requestType
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	bucketSeconds := pickThroughputBucketSeconds(endTime.Sub(startTime))
	data, err := h.opsService.GetErrorTrend(c.Request.Context(), filter, bucketSeconds)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardErrorDistribution returns error distribution by status code (raw path).
// GET /api/v1/admin/ops/dashboard/error-distribution
func (h *OpsHandler) GetDashboardErrorDistribution(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsDashboardFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Platform:  strings.TrimSpace(c.Query("platform")),
		QueryMode: parseOpsQueryMode(c),
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.RequestType = requestType
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	data, err := h.opsService.GetErrorDistribution(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

// GetDashboardPerformance returns model performance KPIs and bounded request points.
// GET /api/v1/admin/ops/dashboard/performance
func (h *OpsHandler) GetDashboardPerformance(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	start, end, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	requestType, err := parseOpsRequestType(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupID, err := parseOpsOptionalPositiveInt64(c.Query("group_id"))
	if err != nil {
		response.BadRequest(c, "Invalid group_id")
		return
	}
	pointLimit := 500
	if raw := strings.TrimSpace(c.Query("point_limit")); raw != "" {
		pointLimit, err = strconv.Atoi(raw)
		if err != nil || pointLimit < 1 || pointLimit > 2000 {
			response.BadRequest(c, "Invalid point_limit")
			return
		}
	}
	filter := &service.OpsPerformanceFilter{StartTime: start, EndTime: end, Platform: strings.TrimSpace(c.Query("platform")), Model: strings.TrimSpace(c.Query("model")), GroupID: groupID, RequestType: requestType, PointLimit: pointLimit}

	var comparison *service.OpsPerformanceFilter
	compare := strings.EqualFold(strings.TrimSpace(c.Query("compare")), "true") || c.Query("compare") == "1"
	comparisonStart, comparisonEnd, comparisonErr := parseComparisonTimeRange(c, start, end)
	if comparisonErr != nil {
		response.BadRequest(c, comparisonErr.Error())
		return
	}
	if compare && comparisonStart.IsZero() && comparisonEnd.IsZero() {
		comparisonStart, comparisonEnd = start.Add(-end.Sub(start)), start
	}
	if compare || !comparisonStart.IsZero() || !comparisonEnd.IsZero() {
		comparison = &service.OpsPerformanceFilter{StartTime: comparisonStart, EndTime: comparisonEnd, Platform: filter.Platform, Model: filter.Model, GroupID: groupID, RequestType: requestType, PointLimit: pointLimit}
	}
	timezone := strings.TrimSpace(c.Query("timezone"))
	if timezone == "" {
		timezone = "UTC"
	}
	data, err := h.opsService.GetPerformanceAnalytics(c.Request.Context(), filter, comparison, timezone)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}

func parseOpsOptionalPositiveInt64(raw string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return nil, fmt.Errorf("invalid positive integer")
	}
	return &value, nil
}

func parseComparisonTimeRange(c *gin.Context, start, end time.Time) (time.Time, time.Time, error) {
	comparisonStartRaw := strings.TrimSpace(c.Query("comparison_start_time"))
	comparisonEndRaw := strings.TrimSpace(c.Query("comparison_end_time"))
	if comparisonStartRaw == "" && comparisonEndRaw == "" {
		return time.Time{}, time.Time{}, nil
	}
	parseTS := func(value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed, nil
		}
		return time.Parse(time.RFC3339, value)
	}
	comparisonStart, err := parseTS(comparisonStartRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid comparison_start_time")
	}
	comparisonEnd, err := parseTS(comparisonEndRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid comparison_end_time")
	}
	if comparisonEnd.IsZero() {
		comparisonEnd = comparisonStart.Add(end.Sub(start))
	}
	if comparisonStart.IsZero() {
		comparisonStart = comparisonEnd.Add(-end.Sub(start))
	}
	if !comparisonStart.Before(comparisonEnd) || comparisonEnd.Sub(comparisonStart) != end.Sub(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("comparison window must have the same positive duration")
	}
	return comparisonStart, comparisonEnd, nil
}

func pickThroughputBucketSeconds(window time.Duration) int {
	// Keep buckets predictable and avoid huge responses.
	switch {
	case window <= 2*time.Hour:
		return 60
	case window <= 24*time.Hour:
		return 300
	default:
		return 3600
	}
}

func parseOpsQueryMode(c *gin.Context) service.OpsQueryMode {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.Query("mode"))
	if raw == "" {
		// Empty means "use server default" (DB setting ops_query_mode_default).
		return ""
	}
	return service.ParseOpsQueryMode(raw)
}

func parseOpsRequestType(c *gin.Context) (*int16, error) {
	if c == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(c.Query("request_type"))
	if raw == "" {
		return nil, nil
	}
	requestType, err := service.ParseUsageRequestType(raw)
	if err != nil {
		return nil, err
	}
	value := int16(requestType)
	return &value, nil
}
