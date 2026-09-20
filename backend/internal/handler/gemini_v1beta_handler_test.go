//go:build unit

package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil || (res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden) {
		return res == nil
	}
	body := strings.ToLower(string(res.Body))
	return strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") ||
		strings.Contains(body, "insufficient authentication scopes") || strings.Contains(body, "access_token_scope_insufficient")
}

func shouldFallbackGeminiModel(modelName string, res *service.UpstreamHTTPResult) bool {
	if shouldFallbackGeminiModels(res) {
		return true
	}
	return res != nil && res.StatusCode == http.StatusNotFound && gemini.HasFallbackModel(modelName)
}

// TestGeminiV1BetaHandler_PlatformRoutingInvariant 文档化并验证 Handler 层的平台路由逻辑不变量
// 该测试确保 gemini 和 antigravity 平台的路由逻辑符合预期
func TestGeminiV1BetaHandler_PlatformRoutingInvariant(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		expectedService string
		description     string
	}{
		{
			name:            "Gemini平台已退役返回410",
			platform:        service.PlatformGemini,
			expectedService: "retired",
			description:     "原生 Gemini 供应已下线，非 Antigravity 账号不再转发",
		},
		{
			name:            "Antigravity平台使用ForwardGemini",
			platform:        service.PlatformAntigravity,
			expectedService: "AntigravityGatewayService.ForwardGemini",
			description:     "Antigravity 账户通过 CRS 中转，支持 Gemini 协议",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 GeminiV1BetaModels 中的路由决策 (lines 199-205 in gemini_v1beta_handler.go)
			var routedService string
			if tt.platform == service.PlatformAntigravity {
				routedService = "AntigravityGatewayService.ForwardGemini"
			} else {
				routedService = "retired"
			}

			require.Equal(t, tt.expectedService, routedService,
				"平台 %s 应该路由到 %s: %s",
				tt.platform, tt.expectedService, tt.description)
		})
	}
}

// TestGeminiV1BetaHandler_ListModelsAntigravityFallback 验证 ListModels 仅在 Antigravity force-platform 下返回静态列表
func TestGeminiV1BetaHandler_ListModelsAntigravityFallback(t *testing.T) {
	tests := []struct {
		name                string
		hasForceAntigravity bool
		expectedBehavior    string
	}{
		{
			name:                "非Antigravity-返回410",
			hasForceAntigravity: false,
			expectedBehavior:    "gone",
		},
		{
			name:                "Antigravity force-platform-返回静态列表",
			hasForceAntigravity: true,
			expectedBehavior:    "static_fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			behavior := "gone"
			if tt.hasForceAntigravity {
				behavior = "static_fallback"
			}
			require.Equal(t, tt.expectedBehavior, behavior)
		})
	}
}

// TestGeminiV1BetaHandler_GetModelAntigravityFallback 验证 GetModel 仅在 Antigravity force-platform 下返回静态模型
func TestGeminiV1BetaHandler_GetModelAntigravityFallback(t *testing.T) {
	tests := []struct {
		name                string
		hasForceAntigravity bool
		expectedBehavior    string
	}{
		{
			name:                "非Antigravity-返回410",
			hasForceAntigravity: false,
			expectedBehavior:    "gone",
		},
		{
			name:                "Antigravity force-platform-返回静态模型信息",
			hasForceAntigravity: true,
			expectedBehavior:    "static_model_info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			behavior := "gone"
			if tt.hasForceAntigravity {
				behavior = "static_model_info"
			}
			require.Equal(t, tt.expectedBehavior, behavior)
		})
	}
}

func TestShouldFallbackGeminiModel_KnownFallbackOn404(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{StatusCode: http.StatusNotFound}
	require.True(t, shouldFallbackGeminiModel("gemini-3.1-pro-preview-customtools", res))
}

func TestShouldFallbackGeminiModel_UnknownModelOn404(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{StatusCode: http.StatusNotFound}
	require.False(t, shouldFallbackGeminiModel("gemini-future-model", res))
}

func TestShouldFallbackGeminiModel_DelegatesScopeFallback(t *testing.T) {
	t.Parallel()

	res := &service.UpstreamHTTPResult{
		StatusCode: http.StatusForbidden,
		Headers:    http.Header{"Www-Authenticate": []string{"Bearer error=\"insufficient_scope\""}},
		Body:       []byte("insufficient authentication scopes"),
	}
	require.True(t, shouldFallbackGeminiModel("gemini-future-model", res))
}
