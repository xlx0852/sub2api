package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

const (
	grokRequestIDHeader     = "X-Grok-Req-Id"
	grokSessionIDHeader     = "X-Grok-Session-Id"
	grokTurnIndexHeader     = "X-Grok-Turn-Idx"
	grokAgentIDHeader       = "X-Grok-Agent-Id"
	grokModelOverrideHeader = "X-Grok-Model-Override"
	grokUpstreamAttemptKey  = "grok_upstream_attempt"
	grokRequestSessionKey   = "grok_request_session_identity"
	grokGatewayAgentID      = "sub2api"
)

func SetGrokUpstreamAttempt(c *gin.Context, attempt int) {
	if c == nil {
		return
	}
	if attempt < 1 {
		attempt = 1
	}
	c.Set(grokUpstreamAttemptKey, attempt)
}

func grokUpstreamAttempt(c *gin.Context) int {
	if c != nil {
		if value, ok := c.Get(grokUpstreamAttemptKey); ok {
			switch typed := value.(type) {
			case int:
				if typed > 0 {
					return typed
				}
			case int64:
				if typed > 0 {
					return int(typed)
				}
			case string:
				if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
					return parsed
				}
			}
		}
	}
	return 1
}

func applyGrokRequestIdentityHeaders(headers http.Header, c *gin.Context, sessionIdentity, upstreamModel string) {
	if headers == nil {
		return
	}
	requestID := grokServerRequestIdentity(c)
	if requestID != "" {
		headers.Set(grokRequestIDHeader, requestID)
	} else {
		headers.Del(grokRequestIDHeader)
	}
	sessionIdentity = grokRequestSessionIdentity(c, sessionIdentity)
	if sessionIdentity != "" {
		headers.Set(grokSessionIDHeader, sessionIdentity)
	} else {
		headers.Del(grokSessionIDHeader)
	}
	headers.Set(grokAgentIDHeader, grokGatewayAgentID)
	headers.Set(grokTurnIndexHeader, strconv.Itoa(grokUpstreamAttempt(c)))
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel != "" {
		headers.Set(grokModelOverrideHeader, upstreamModel)
	} else {
		headers.Del(grokModelOverrideHeader)
	}
}

func rememberGrokRequestSessionIdentity(c *gin.Context, apiKeyID int64, seed string) {
	if c == nil || apiKeyID <= 0 || strings.TrimSpace(seed) == "" {
		return
	}
	if existing, ok := c.Get(grokRequestSessionKey); ok {
		if value, ok := existing.(string); ok && strings.TrimSpace(value) != "" {
			return
		}
	}
	c.Set(grokRequestSessionKey, generateSessionUUID(
		"grok-request-session:v1:"+strconv.FormatInt(apiKeyID, 10)+":"+strings.TrimSpace(seed),
	))
}

func grokRequestSessionIdentity(c *gin.Context, fallback string) string {
	if c != nil {
		if value, ok := c.Get(grokRequestSessionKey); ok {
			if session, ok := value.(string); ok && strings.TrimSpace(session) != "" {
				return strings.TrimSpace(session)
			}
		}
	}
	return strings.TrimSpace(fallback)
}

func grokServerRequestIdentity(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	ctx := c.Request.Context()
	raw := contextStringValue(ctx, ctxkey.ClientRequestID)
	if raw == "" {
		raw = contextStringValue(ctx, ctxkey.RequestID)
	}
	if raw == "" {
		return ""
	}
	return generateSessionUUID("grok-request:v1:" + raw)
}

func contextStringValue(ctx context.Context, key ctxkey.Key) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key).(string)
	return strings.TrimSpace(value)
}
