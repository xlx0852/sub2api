package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func upstreamModelSyncTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
}

func TestBuildV1ModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1/models"))
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", buildV1ModelsURL("https://gateway.example.com/antigravity/"))
}

func TestBuildOpenAIModelsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "zhipu v4 coding base url",
			base: "https://open.bigmodel.cn/api/coding/paas/v4",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
		},
		{
			name: "openai v1 base url",
			base: "https://api.openai.com/v1",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "models url unchanged",
			base: "https://api.openai.com/v1/models",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "host fallback uses v1",
			base: "https://api.openai.com",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "trailing slash on v4",
			base: "https://open.bigmodel.cn/api/coding/paas/v4/",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
		},
		{
			name: "v2 base url",
			base: "https://gateway.example.com/openai/v2",
			want: "https://gateway.example.com/openai/v2/models",
		},
		{
			name: "v3 base url",
			base: "https://gateway.example.com/openai/v3",
			want: "https://gateway.example.com/openai/v3/models",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, buildOpenAIModelsURL(tt.base))
		})
	}
}

func TestBuildGeminiModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta/models"))
}

func TestExtractUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "openai and anthropic data array",
			body: `{"data":[{"id":"claude-sonnet-4-5"},{"id":"gpt-5"},{"id":"gpt-5"},{"id":""}]}`,
			want: []string{"claude-sonnet-4-5", "gpt-5"},
		},
		{
			name: "gemini models array strips prefix",
			body: `{"models":[{"name":"models/gemini-2.5-pro"},{"name":"gemini-2.5-flash"}]}`,
			want: []string{"gemini-2.5-flash", "gemini-2.5-pro"},
		},
		{
			name: "top level array",
			body: `[{"id":"z-model"},{"name":"models/a-model"}]`,
			want: []string{"a-model", "z-model"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractUpstreamModelIDs([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildUpstreamModelsRequestsForAPIKeyAccounts(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	ctx := context.Background()

	anthropicReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "anthropic-key",
			"base_url": "https://anthropic.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://anthropic.example.com/v1/models", anthropicReq.URL.String())
	require.Equal(t, "anthropic-key", anthropicReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicReq.Header.Get("anthropic-version"))

	anthropicBearerReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "ollama-key",
			"base_url": "https://ollama.com",
		},
		Extra: map[string]any{
			"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/models", anthropicBearerReq.URL.String())
	require.Equal(t, "Bearer ollama-key", anthropicBearerReq.Header.Get("Authorization"))
	require.Empty(t, anthropicBearerReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicBearerReq.Header.Get("anthropic-version"))

	openAIReq, err := svc.buildOpenAIUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://openai.example.com/v1/models", openAIReq.URL.String())
	require.Equal(t, "Bearer openai-key", openAIReq.Header.Get("Authorization"))

	grokReq, err := svc.buildGrokUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/models", grokReq.URL.String())
	require.Equal(t, "Bearer xai-key", grokReq.Header.Get("Authorization"))

	geminiReq, err := svc.buildGeminiUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "gemini-key",
			"base_url": "https://generativelanguage.googleapis.com/v1beta",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", geminiReq.URL.String())
	require.Equal(t, "gemini-key", geminiReq.Header.Get("x-goog-api-key"))

	antigravityReq, err := svc.buildAntigravityAPIKeyModelsRequest(ctx, &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "antigravity-key",
			"base_url": "https://gateway.example.com/antigravity",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", antigravityReq.URL.String())
	require.Equal(t, "antigravity-key", antigravityReq.Header.Get("x-api-key"))
}

func TestBuildGrokUpstreamModelsRequestOAuthRequiresOfficialBaseURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var syncErr *UpstreamModelSyncError

	// 自定义 base_url：OAuth 订阅凭据不得离开官方 host。
	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig(), grokTokenProvider: &GrokTokenProvider{}}
	_, err := svc.buildGrokUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"base_url":     "https://grok-relay.example.com/v1",
		},
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.NotContains(t, syncErr.SafeMessage(), "oauth-token")

	// 未注入 token provider：配置问题，不是上游故障。
	svc = &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err = svc.buildGrokUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"base_url":     "https://api.x.ai/v1",
		},
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)

	// 过期且无 refresh_token：在发出任何请求前就报配置错误。
	svc = &AccountTestService{cfg: upstreamModelSyncTestConfig(), grokTokenProvider: &GrokTokenProvider{}}
	_, err = svc.buildGrokUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"base_url":     "https://api.x.ai/v1",
			"expires_at":   time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		},
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)

	// 非 API key / 非 OAuth 的类型仍然 unsupported，消息不变。
	svc = &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err = svc.buildGrokUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeServiceAccount,
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "Unsupported Grok account type")
}

func TestBuildGrokUpstreamModelsRequestUsesOAuthAccessToken(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig(), grokTokenProvider: &GrokTokenProvider{}}
	req, err := svc.buildGrokUpstreamModelsRequest(context.Background(), &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"base_url":     "https://api.x.ai/v1",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/models", req.URL.String())
	require.Equal(t, "Bearer oauth-token", req.Header.Get("Authorization"))
}

func TestExtractCodexManifestModelIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "hides internal models and prefers slug",
			body: `{"models":[{"slug":"gpt-5.5","id":"gpt-5.5-2026","visibility":"list"},{"slug":"gpt-6.1-sol"},{"slug":"gpt-reserve","visibility":"hide"},{"slug":"codex-auto-review","visibility":"HIDE"}]}`,
			want: []string{"gpt-5.5", "gpt-6.1-sol"},
		},
		{
			name: "falls back to id and keeps unknown visibility",
			body: `{"models":[{"id":"gpt-6-sol","visibility":"preview"},{"slug":"gpt-6-luna","visibility":""}]}`,
			want: []string{"gpt-6-luna", "gpt-6-sol"},
		},
		{
			name: "dedupes and drops empty entries",
			body: `{"models":[{"slug":"gpt-5.5"},{"slug":"gpt-5.5"},{"slug":"  "},{"slug":""}]}`,
			want: []string{"gpt-5.5"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractCodexManifestModelIDs([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	_, err := extractCodexManifestModelIDs([]byte(`not json`))
	require.Error(t, err)
}

func TestFetchUpstreamSupportedModelsOpenAIOAuthUsesCodexManifest(t *testing.T) {
	// 该用例改写包级 chatgptCodexModelsURL，因此不能并行。
	var gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClientVersion = r.URL.Query().Get("client_version")
		require.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-6.1-sol","visibility":"list"},{"slug":"gpt-reserve","visibility":"hide"},{"slug":"gpt-5.5"},{"slug":"gpt-6.1-sol"}]}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	// settingService 的 settingRepo 为 nil → 回退到内置编译常量，正好用来断言
	// 「同步用的版本与网关对外声明的版本同源」这一段接线。
	svc := &AccountTestService{
		cfg: upstreamModelSyncTestConfig(),
		openAIGatewayService: &OpenAIGatewayService{
			settingService: &SettingService{},
		},
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       69,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acc-123",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.5", "gpt-6.1-sol"}, models)
	require.Equal(t, codexCLIVersion, gotClientVersion)
}

func TestFetchUpstreamSupportedModelsOpenAIOAuthCredentialFailures(t *testing.T) {
	t.Parallel()
	var syncErr *UpstreamModelSyncError

	// gateway service 未注入（旧构造路径 / 测试装配）→ configuration，不得 panic。
	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       69,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "test-access-token",
		},
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)

	// 无可用凭据（不可刷新）→ configuration，且不发任何上游请求。
	upstream := &httpUpstreamRecorder{}
	svc = &AccountTestService{
		cfg:                  upstreamModelSyncTestConfig(),
		openAIGatewayService: &OpenAIGatewayService{},
		httpUpstream:         upstream,
	}
	_, err = svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       113,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "acc-123",
		},
	})
	require.Error(t, err)
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	require.Nil(t, upstream.lastReq)
}

func TestFetchUpstreamSupportedModelsKeepsRetiredCombinationsRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		account *Account
		kind    UpstreamModelSyncErrorKind
	}{
		{
			name:    "anthropic oauth is retired",
			account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			kind:    UpstreamModelSyncErrorConfiguration,
		},
		{
			name:    "anthropic setup token is retired",
			account: &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeSetupToken},
			kind:    UpstreamModelSyncErrorConfiguration,
		},
		{
			name:    "anthropic service account is unsupported",
			account: &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount},
			kind:    UpstreamModelSyncErrorUnsupported,
		},
		{
			name: "gemini oauth with project id is unsupported",
			account: &Account{ID: 4, Platform: PlatformGemini, Type: AccountTypeOAuth,
				Credentials: map[string]any{"project_id": "proj-1"}},
			kind: UpstreamModelSyncErrorUnsupported,
		},
		{
			name:    "gemini service account is unsupported",
			account: &Account{ID: 5, Platform: PlatformGemini, Type: AccountTypeServiceAccount},
			kind:    UpstreamModelSyncErrorUnsupported,
		},
		{
			name:    "kimi oauth has no listing path",
			account: &Account{ID: 6, Platform: PlatformKimi, Type: AccountTypeOAuth},
			kind:    UpstreamModelSyncErrorUnsupported,
		},
		{
			name:    "openai service account is unsupported",
			account: &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeServiceAccount},
			kind:    UpstreamModelSyncErrorUnsupported,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream := &httpUpstreamRecorder{}
			svc := &AccountTestService{cfg: upstreamModelSyncTestConfig(), httpUpstream: upstream}

			models, err := svc.FetchUpstreamSupportedModels(context.Background(), tc.account)
			require.Error(t, err)
			require.Empty(t, models)
			var syncErr *UpstreamModelSyncError
			require.True(t, errors.As(err, &syncErr))
			require.Equal(t, tc.kind, syncErr.Kind)
			// 退役/未验证组合必须在发请求前就失败。
			require.Nil(t, upstream.lastReq)
		})
	}
}

func TestFetchUpstreamSupportedModelsOpenAIOAuthUpstreamFailures(t *testing.T) {
	// 改写包级 chatgptCodexModelsURL，因此不能并行。
	cases := []struct {
		name       string
		status     int
		body       string
		wantMsg    string
		leakString string
	}{
		{
			name:       "upstream http error",
			status:     http.StatusInternalServerError,
			body:       `boom-secret-payload`,
			wantMsg:    "Failed to fetch OpenAI Codex models manifest",
			leakString: "boom-secret-payload",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    `not json`,
			wantMsg: "Upstream model list response was not valid JSON",
		},
		{
			name:    "empty model list",
			status:  http.StatusOK,
			body:    `{"models":[]}`,
			wantMsg: "Upstream returned no supported models",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			original := chatgptCodexModelsURL
			chatgptCodexModelsURL = server.URL
			defer func() { chatgptCodexModelsURL = original }()

			svc := &AccountTestService{
				cfg: upstreamModelSyncTestConfig(),
				openAIGatewayService: &OpenAIGatewayService{
					settingService: &SettingService{},
				},
			}

			models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
				ID:       69,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token":       "test-access-token",
					"chatgpt_account_id": "acc-123",
				},
			})
			require.Error(t, err)
			require.Empty(t, models)
			var syncErr *UpstreamModelSyncError
			require.True(t, errors.As(err, &syncErr))
			require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
			require.Equal(t, tc.wantMsg, syncErr.SafeMessage())
			if tc.leakString != "" {
				// 上游正文只能进内部 err，不能进客户端可见消息。
				require.NotContains(t, syncErr.SafeMessage(), tc.leakString)
			}
		})
	}
}

func TestFetchUpstreamSupportedModelsParsesGrokResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.5"},{"id":"grok-4.5"},{"id":"grok-imagine"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://api.x.ai/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-imagine"}, models)
	require.Equal(t, "https://api.x.ai/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-key", upstream.lastReq.Header.Get("Authorization"))
}

func TestBuildAntigravityAPIKeyModelsRequestRejectsOfficialCloudCodeBase(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err := svc.buildAntigravityAPIKeyModelsRequest(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "antigravity-key",
			"base_url": "https://cloudcode-pa.googleapis.com",
		},
	})
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "compatible gateway")
}

func TestFetchUpstreamSupportedModelsParsesOpenAIResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"},{"id":"gpt-5"},{"name":"o3"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5", "o3"}, models)
	require.Equal(t, "https://openai.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer openai-key", upstream.lastReq.Header.Get("Authorization"))
}

func TestFetchUpstreamSupportedModelsDoesNotExposeUpstreamBody(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"SECRET_TOKEN should not be exposed"}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
		},
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRET_TOKEN")

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.NotContains(t, syncErr.SafeMessage(), "SECRET_TOKEN")
	require.Contains(t, syncErr.SafeMessage(), "HTTP 502")
}
