package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const errorPassthroughServiceContextKey = "error_passthrough_service"

// BindErrorPassthroughService 将错误透传服务绑定到请求上下文，供 service 层在非 failover 场景下复用规则。
func BindErrorPassthroughService(c *gin.Context, svc *ErrorPassthroughService) {
	if c == nil || svc == nil {
		return
	}
	c.Set(errorPassthroughServiceContextKey, svc)
}

func getBoundErrorPassthroughService(c *gin.Context) *ErrorPassthroughService {
	if c == nil {
		return nil
	}
	v, ok := c.Get(errorPassthroughServiceContextKey)
	if !ok {
		return nil
	}
	svc, ok := v.(*ErrorPassthroughService)
	if !ok {
		return nil
	}
	return svc
}

// applyErrorPassthroughRule 按规则改写错误响应；未命中时返回默认响应参数。
func applyErrorPassthroughRule(
	c *gin.Context,
	platform string,
	upstreamStatus int,
	responseBody []byte,
	defaultStatus int,
	defaultErrType string,
	defaultErrMsg string,
) (status int, errType string, errMsg string, matched bool) {
	status = defaultStatus
	errType = defaultErrType
	errMsg = defaultErrMsg

	svc := getBoundErrorPassthroughService(c)
	if svc == nil {
		return status, errType, errMsg, false
	}

	rule := svc.MatchRule(platform, upstreamStatus, responseBody)
	if rule == nil {
		return status, errType, errMsg, false
	}

	status = upstreamStatus
	if !rule.PassthroughCode && rule.ResponseCode != nil {
		status = *rule.ResponseCode
	}

	errMsg = ExtractUpstreamErrorMessage(responseBody)
	if !rule.PassthroughBody && rule.CustomMessage != nil {
		errMsg = *rule.CustomMessage
	}
	// Cloudflare HTML 5xx pages must not surface as empty/raw client messages,
	// and should not force the original 524/504 status when body is not JSON.
	if looksLikeHTMLUpstreamErrorBody(responseBody) {
		if mappedStatus, mappedType, mappedMsg := MapOpenAIUpstreamErrorStatus(upstreamStatus); mappedStatus > 0 {
			if rule.PassthroughCode || status == upstreamStatus {
				status = mappedStatus
			}
			errType = mappedType
			if strings.TrimSpace(errMsg) == "" || looksLikeHTMLUpstreamErrorBody([]byte(errMsg)) {
				errMsg = mappedMsg
				if summarized := summarizeNonJSONUpstreamErrorBody(responseBody); summarized != "" {
					errMsg = summarized
				}
			}
		}
	}
	if strings.TrimSpace(errMsg) == "" {
		errMsg = defaultErrMsg
	}

	// 命中 skip_monitoring 时在 context 中标记，供 ops_error_logger 跳过记录。
	if rule.SkipMonitoring {
		c.Set(OpsSkipPassthroughKey, true)
	}

	// 与现有 failover 场景保持一致：命中规则时统一返回 upstream_error。
	if errType == "" {
		errType = "upstream_error"
	}
	return status, errType, errMsg, true
}
