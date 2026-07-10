package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex models-manager (codex-rs) refreshes the picker only under ChatGPT auth,
// via ModelsClient.list_models → GET {provider.base_url}/models?client_version=...
// with a hard 5s timeout, decoding ModelsResponse {"models":[ModelInfo...]}.
// Common gateway entrypoints that land here:
//   - GET /models?client_version=...              (custom provider base_url root)
//   - GET /v1/models?client_version=...           (OpenAI-compatible base_url)
//   - GET /backend-api/codex/models?client_version=... (ChatGPT backend base)
// The body is proxied verbatim from chatgpt.com using a priority-ordered OAuth
// probe (TokenProvider-backed): up to a few schedulable accounts, early-exit
// once a gpt-5.6* family catalog is found. This is a best-effort "rich enough"
// selection under the CLI 5s timeout, not a strict global richest scan.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		if h != nil {
			h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "API key group is required"}})
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI {
		if h != nil {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI groups")
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found_error", "message": "Codex models manifest is only available for OpenAI groups"}})
		return
	}
	if h == nil || h.gatewayService == nil {
		if h != nil {
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "gateway service unavailable")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "gateway service unavailable",
			},
		})
		return
	}

	manifest, err := h.gatewayService.FetchPreferredCodexModelsManifest(c.Request.Context(), apiKey.GroupID, c.Query("client_version"), c.GetHeader("If-None-Match"))
	if err != nil {
		h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
		return
	}

	if manifest.ETag != "" {
		c.Header("ETag", manifest.ETag)
	}
	if manifest.NotModified {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json", manifest.Body)
}
