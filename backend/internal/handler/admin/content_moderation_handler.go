package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ContentModerationHandler struct {
	service       *service.ContentModerationService
	retrospective *service.RetrospectiveRiskService
}

func NewContentModerationHandler(svc *service.ContentModerationService, retrospective *service.RetrospectiveRiskService) *ContentModerationHandler {
	return &ContentModerationHandler{service: svc, retrospective: retrospective}
}

type contentModerationConfigRequest struct {
	Enabled                  *bool               `json:"enabled"`
	Mode                     *string             `json:"mode"`
	BaseURL                  *string             `json:"base_url"`
	Model                    *string             `json:"model"`
	APIKey                   *string             `json:"api_key"`
	APIKeys                  *[]string           `json:"api_keys"`
	APIKeysMode              string              `json:"api_keys_mode"`
	DeleteAPIKeyHashes       *[]string           `json:"delete_api_key_hashes"`
	ClearAPIKey              bool                `json:"clear_api_key"`
	TimeoutMS                *int                `json:"timeout_ms"`
	SampleRate               *int                `json:"sample_rate"`
	AllGroups                *bool               `json:"all_groups"`
	GroupIDs                 *[]int64            `json:"group_ids"`
	RecordNonHits            *bool               `json:"record_non_hits"`
	Thresholds               *map[string]float64 `json:"thresholds"`
	WorkerCount              *int                `json:"worker_count"`
	QueueSize                *int                `json:"queue_size"`
	BlockStatus              *int                `json:"block_status"`
	BlockMessage             *string             `json:"block_message"`
	EmailOnHit               *bool               `json:"email_on_hit"`
	AutoDisableAPIKeyEnabled *bool               `json:"auto_disable_api_key_enabled"`
	AutoBanEnabled           *bool               `json:"auto_ban_enabled"`
	BanThreshold             *int                `json:"ban_threshold"`
	ViolationWindowHours     *int                `json:"violation_window_hours"`
	// cyber_policy 命中是否排除出自动封号计数；前端 RiskControlView 已发送该字段，
	// service.UpdateContentModerationConfigInput 已支持，此前 handler 层缺透传导致开关静默失效。
	CyberPolicyExcludeFromBanCount *bool                                 `json:"cyber_policy_exclude_from_ban_count"`
	RetryCount                     *int                                  `json:"retry_count"`
	HitRetentionDays               *int                                  `json:"hit_retention_days"`
	NonHitRetentionDays            *int                                  `json:"non_hit_retention_days"`
	PreHashCheckEnabled            *bool                                 `json:"pre_hash_check_enabled"`
	BlockedKeywords                *[]string                             `json:"blocked_keywords"`
	KeywordBlockingMode            *string                               `json:"keyword_blocking_mode"`
	ModelFilter                    *service.ContentModerationModelFilter `json:"model_filter"`
	Retrospective                  *service.RetrospectiveRiskConfig      `json:"retrospective"`
}

type contentModerationAPIKeyTestRequest struct {
	APIKeys   []string `json:"api_keys"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model"`
	TimeoutMS int      `json:"timeout_ms"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
}

type contentModerationHashRequest struct {
	InputHash string `json:"input_hash"`
}

func (h *ContentModerationHandler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) UpdateConfig(c *gin.Context) {
	var req contentModerationConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.UpdateContentModerationConfigInput{
		Enabled:                        req.Enabled,
		Mode:                           req.Mode,
		BaseURL:                        req.BaseURL,
		Model:                          req.Model,
		APIKey:                         req.APIKey,
		APIKeys:                        req.APIKeys,
		APIKeysMode:                    req.APIKeysMode,
		DeleteAPIKeyHashes:             req.DeleteAPIKeyHashes,
		ClearAPIKey:                    req.ClearAPIKey,
		TimeoutMS:                      req.TimeoutMS,
		SampleRate:                     req.SampleRate,
		AllGroups:                      req.AllGroups,
		GroupIDs:                       req.GroupIDs,
		RecordNonHits:                  req.RecordNonHits,
		Thresholds:                     req.Thresholds,
		WorkerCount:                    req.WorkerCount,
		QueueSize:                      req.QueueSize,
		BlockStatus:                    req.BlockStatus,
		BlockMessage:                   req.BlockMessage,
		EmailOnHit:                     req.EmailOnHit,
		AutoDisableAPIKeyEnabled:       req.AutoDisableAPIKeyEnabled,
		AutoBanEnabled:                 req.AutoBanEnabled,
		BanThreshold:                   req.BanThreshold,
		ViolationWindowHours:           req.ViolationWindowHours,
		CyberPolicyExcludeFromBanCount: req.CyberPolicyExcludeFromBanCount,
		RetryCount:                     req.RetryCount,
		HitRetentionDays:               req.HitRetentionDays,
		NonHitRetentionDays:            req.NonHitRetentionDays,
		PreHashCheckEnabled:            req.PreHashCheckEnabled,
		BlockedKeywords:                req.BlockedKeywords,
		KeywordBlockingMode:            req.KeywordBlockingMode,
		ModelFilter:                    req.ModelFilter,
		Retrospective:                  req.Retrospective,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) GetRetrospectiveStatus(c *gin.Context) {
	if h.retrospective == nil {
		response.Success(c, map[string]any{"configured": false})
		return
	}
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	status, err := h.retrospective.Status(c.Request.Context(), cfg.Retrospective)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ContentModerationHandler) TriggerRetrospectiveReview(c *gin.Context) {
	if h.retrospective == nil {
		response.BadRequest(c, "Retrospective review is unavailable")
		return
	}
	response.Success(c, h.retrospective.TriggerReview())
}

func (h *ContentModerationHandler) ListRetrospectiveReviewRuns(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.retrospective.ListRuns(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *ContentModerationHandler) ListRiskProfiles(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.retrospective.ListProfiles(c.Request.Context(), c.Query("level"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *ContentModerationHandler) ListRiskEvents(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		userID = parsed
	}
	items, total, err := h.retrospective.ListEvents(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *ContentModerationHandler) ListRetrospectiveEvidenceCases(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		userID = parsed
	}
	items, total, err := h.retrospective.ListEvidenceCases(c.Request.Context(), userID, c.Query("status"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

type retrospectiveEvidenceAuthorizeRequest struct {
	Password string `json:"password" binding:"required"`
	Reason   string `json:"reason" binding:"required"`
}

func (h *ContentModerationHandler) AuthorizeRetrospectiveEvidence(c *gin.Context) {
	var req retrospectiveEvidenceAuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.BadRequest(c, "Missing admin identity")
		return
	}
	grant, err := h.retrospective.AuthorizeEvidenceAccess(c.Request.Context(), strings.TrimSpace(c.Param("evidence_id")), subject.UserID, req.Password, req.Reason)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, grant)
}

func (h *ContentModerationHandler) GetRawRetrospectiveEvidence(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.BadRequest(c, "Missing admin identity")
		return
	}
	token := strings.TrimSpace(c.GetHeader("X-Evidence-Access-Token"))
	result, err := h.retrospective.ReadRawEvidence(c.Request.Context(), strings.TrimSpace(c.Param("evidence_id")), subject.UserID, token)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

type retrospectiveProfileReviewRequest struct {
	Level        string `json:"level"`
	ReviewStatus string `json:"review_status"`
	Note         string `json:"note"`
}

func (h *ContentModerationHandler) ReviewRiskProfile(c *gin.Context) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	var req retrospectiveProfileReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	var adminID *int64
	if ok && subject.UserID > 0 {
		id := subject.UserID
		adminID = &id
	}
	profile, err := h.retrospective.ReviewProfile(c.Request.Context(), userID, strings.TrimSpace(req.Level), strings.TrimSpace(req.ReviewStatus), strings.TrimSpace(req.Note), adminID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, profile)
}

func (h *ContentModerationHandler) TestAPIKeys(c *gin.Context) {
	var req contentModerationAPIKeyTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.TestAPIKeys(c.Request.Context(), service.TestContentModerationAPIKeysInput{
		APIKeys:   req.APIKeys,
		BaseURL:   req.BaseURL,
		Model:     req.Model,
		TimeoutMS: req.TimeoutMS,
		Prompt:    req.Prompt,
		Images:    req.Images,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ContentModerationHandler) ListLogs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Result:   c.Query("result"),
		Endpoint: c.Query("endpoint"),
		Search:   c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	summary, err := h.service.SummarizeLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, struct {
		Items    []service.ContentModerationLog       `json:"items"`
		Total    int64                                `json:"total"`
		Page     int                                  `json:"page"`
		PageSize int                                  `json:"page_size"`
		Pages    int                                  `json:"pages"`
		Summary  *service.ContentModerationLogSummary `json:"summary"`
	}{
		Items: items, Total: pageResult.Total, Page: pageResult.Page,
		PageSize: pageResult.PageSize, Pages: pageResult.Pages, Summary: summary,
	})
}

func (h *ContentModerationHandler) UnbanUser(c *gin.Context) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	result, err := h.service.UnbanUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) DeleteFlaggedHash(c *gin.Context) {
	var req contentModerationHashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.DeleteFlaggedInputHash(c.Request.Context(), req.InputHash)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) ClearFlaggedHashes(c *gin.Context) {
	result, err := h.service.ClearFlaggedInputHashes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseContentModerationDate(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
