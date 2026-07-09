package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Compact inbound protocol modes for Codex remote compaction.
const (
	OpenAICompactModeNone         = ""
	OpenAICompactModeLegacyPath   = "legacy_path"
	OpenAICompactModeBodySignalV2 = "body_signal_v2"
)

const (
	openAICompactModeContextKey             = "openai_compact_mode"
	openAICompactForceUpstreamSuffixKey     = "openai_compact_force_upstream_suffix"
	openAICompactBridgeUsedContextKey       = "openai_compact_bridge_used"
	openAICompactTerminalCommittedContextKey = "openai_compact_terminal_committed"
	openAICompactV2SSEStartedContextKey     = "openai_compact_v2_sse_started"
)

// SetOpenAICompactMode stores the compact protocol mode for the current request.
func SetOpenAICompactMode(c *gin.Context, mode string) {
	if c == nil {
		return
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return
	}
	c.Set(openAICompactModeContextKey, mode)
}

// OpenAICompactMode returns the compact protocol mode for the current request.
func OpenAICompactMode(c *gin.Context) string {
	if c == nil {
		return OpenAICompactModeNone
	}
	if v, ok := c.Get(openAICompactModeContextKey); ok {
		if mode, ok := v.(string); ok {
			return strings.TrimSpace(mode)
		}
	}
	return OpenAICompactModeNone
}

// IsOpenAICompactRequest reports whether the request is any Codex compact mode.
func IsOpenAICompactRequest(c *gin.Context) bool {
	mode := OpenAICompactMode(c)
	if mode == OpenAICompactModeLegacyPath || mode == OpenAICompactModeBodySignalV2 {
		return true
	}
	// Fallback for callers that only rewrote the path historically.
	return isOpenAIResponsesCompactPath(c)
}

// IsOpenAIBodySignalCompactV2 reports remote compaction v2 body-signal mode.
func IsOpenAIBodySignalCompactV2(c *gin.Context) bool {
	return OpenAICompactMode(c) == OpenAICompactModeBodySignalV2
}

// SetOpenAICompactForceUpstreamSuffix forces upstream path suffix (e.g. "/compact")
// while keeping the client-facing request path unchanged.
func SetOpenAICompactForceUpstreamSuffix(c *gin.Context, suffix string) {
	if c == nil {
		return
	}
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	c.Set(openAICompactForceUpstreamSuffixKey, suffix)
}

func openAICompactForceUpstreamSuffix(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(openAICompactForceUpstreamSuffixKey); ok {
		if suffix, ok := v.(string); ok {
			return strings.TrimSpace(suffix)
		}
	}
	return ""
}

// MarkOpenAICompactBridgeUsed records that legacy JSON was bridged to v2 SSE.
func MarkOpenAICompactBridgeUsed(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAICompactBridgeUsedContextKey, true)
}

// IsOpenAICompactBridgeUsed reports whether the SSE bridge was used.
func IsOpenAICompactBridgeUsed(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(openAICompactBridgeUsedContextKey); ok {
		if used, ok := v.(bool); ok {
			return used
		}
	}
	return false
}

// MarkOpenAICompactTerminalCommitted marks that terminal compact output was written
// to the client (JSON body or SSE terminal events). Soft-timeout retry is blocked after this.
func MarkOpenAICompactTerminalCommitted(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAICompactTerminalCommittedContextKey, true)
}

// IsOpenAICompactTerminalCommitted reports terminal compact output commitment.
func IsOpenAICompactTerminalCommitted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(openAICompactTerminalCommittedContextKey); ok {
		if committed, ok := v.(bool); ok {
			return committed
		}
	}
	return false
}

// MarkOpenAICompactV2SSEStarted marks that body-signal v2 downstream SSE headers/keepalives started.
func MarkOpenAICompactV2SSEStarted(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAICompactV2SSEStartedContextKey, true)
}

// IsOpenAICompactV2SSEStarted reports whether v2 SSE has started on the client connection.
func IsOpenAICompactV2SSEStarted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(openAICompactV2SSEStartedContextKey); ok {
		if started, ok := v.(bool); ok {
			return started
		}
	}
	return false
}

// CanRetryOpenAICompactAfterForwardError reports whether account switch is still safe.
// Body-signal v2 keepalives alone do not block retry; terminal events do.
func CanRetryOpenAICompactAfterForwardError(c *gin.Context, writerSizeBeforeForward int) bool {
	if IsOpenAICompactTerminalCommitted(c) {
		return false
	}
	if IsOpenAIBodySignalCompactV2(c) && IsOpenAICompactV2SSEStarted(c) {
		return true
	}
	if c == nil || c.Writer == nil {
		return true
	}
	return c.Writer.Size() == writerSizeBeforeForward
}
