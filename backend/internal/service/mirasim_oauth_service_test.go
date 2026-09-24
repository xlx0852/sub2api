//go:build unit

package service

// Mirasim 授权登录（OAuth onboarding）单测：覆盖「凭据抽取 + 本地 JWT 校验 + 输入
// 归一化 + 上游错误归类」这些不需要联网的部分。
//
// 关键约束（改动前请先读）：
//   - 绝不调用真实 /auth/code、/auth/verify、/auth/refresh（会真发邮件 / 消耗一次性
//     验证码 / 轮换 refresh token 把用户本地客户端踢下线）。这里只做纯本地函数与
//     假 HTTPUpstream 的断言。
//   - 测试里的 token 全部是本地现造的假 JWT，绝不使用任何真实凭据。
//   - 错误断言只看 ApplicationError 的 HTTP 状态（Code）与业务码（Reason），
//     不匹配完整文案。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// mirasimTestJWT 现造一个三段 JWT（不验签，只求 payload 可解析）。
func mirasimTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + "." + base64.RawURLEncoding.EncodeToString([]byte("signature"))
}

// mirasimTestAppError 取出 ApplicationError，并断言状态码与业务码。
func mirasimTestAppError(t *testing.T, err error, wantStatus int, wantReason string) {
	t.Helper()
	require.Error(t, err)
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, wantReason, appErr.Reason)
	require.Equal(t, int32(wantStatus), appErr.Code)
}

func TestMirasimOAuthExtractRefreshTokenFromCallbackURL(t *testing.T) {
	token := mirasimTestJWT(t, map[string]any{"email": "user@example.com", "exp": 4102444800})

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "完整回调链接 query 参数",
			raw:  "http://127.0.0.1:9/callback?refresh_token=" + token + "&state=abc",
			want: token,
		},
		{
			name: "裸参数串（只复制了 ? 之后的部分）",
			raw:  "?refresh_token=" + token,
			want: token,
		},
		{
			name: "hash 片段参数",
			raw:  "http://127.0.0.1:9/callback#refresh_token=" + token,
			want: token,
		},
		{
			name: "直接粘贴 token 原文",
			raw:  token,
			want: token,
		},
		{
			name: "两侧空白被裁剪",
			raw:  "  " + token + "\n",
			want: token,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractMirasimRefreshTokenFromCallbackURL(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMirasimOAuthExtractRefreshTokenRejects(t *testing.T) {
	// 注意：回调 URL 里含明文 refresh token，这里只用假 token 构造。
	cases := []struct {
		name       string
		raw        string
		wantReason string
	}{
		{"空串", "   ", "MIRASIM_OAUTH_CALLBACK_REQUIRED"},
		{
			name:       "链接里没有 refresh_token",
			raw:        "http://127.0.0.1:9/callback?code=abc&state=xyz",
			wantReason: "MIRASIM_OAUTH_CALLBACK_NO_TOKEN",
		},
		{
			name:       "既不是链接也不是 JWT",
			raw:        "hello world",
			wantReason: "MIRASIM_OAUTH_CALLBACK_NO_TOKEN",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ExtractMirasimRefreshTokenFromCallbackURL(tc.raw)
			// 解析类错误一律 400，且不影响上游（无网络调用）。
			mirasimTestAppError(t, err, http.StatusBadRequest, tc.wantReason)
		})
	}
}

func TestMirasimOAuthParseRefreshToken(t *testing.T) {
	future := time.Now().Add(24 * time.Hour).Unix()
	identity, err := ParseMirasimRefreshToken(mirasimTestJWT(t, map[string]any{
		"email":      "Someone@Example.com",
		"name":       "Someone",
		"plan":       "pro",
		"plan_exp":   4102444800,
		"exp":        future,
		"token_type": "refresh",
		"iss":        "mirofish",
	}))
	require.NoError(t, err)
	// 与 /auth/oauth 路径一致：email 原样保留（不强制小写），避免身份串味。
	require.Equal(t, "Someone@Example.com", identity.Email)
	require.Equal(t, "Someone", identity.Name)
	require.Equal(t, "pro", identity.Plan)
	require.Equal(t, int64(4102444800), identity.PlanExp)
	require.Equal(t, future, identity.ExpiresAt)

	t.Run("缺 email 时退回 sub", func(t *testing.T) {
		identity, err := ParseMirasimRefreshToken(mirasimTestJWT(t, map[string]any{
			"sub": "fallback@example.com",
			"exp": future,
		}))
		require.NoError(t, err)
		require.Equal(t, "fallback@example.com", identity.Email)
	})

	t.Run("数值 claim 用字符串也兼容", func(t *testing.T) {
		identity, err := ParseMirasimRefreshToken(mirasimTestJWT(t, map[string]any{
			"email": "user@example.com",
			"exp":   "4102444800",
		}))
		require.NoError(t, err)
		require.Equal(t, int64(4102444800), identity.ExpiresAt)
	})
}

func TestMirasimOAuthParseRefreshTokenRejects(t *testing.T) {
	past := time.Now().Add(-time.Hour).Unix()
	cases := []struct {
		name       string
		token      string
		wantReason string
	}{
		{"空", "  ", "MIRASIM_OAUTH_TOKEN_REQUIRED"},
		{"不是三段", "only.two", "MIRASIM_OAUTH_TOKEN_MALFORMED"},
		{"payload 非 base64", "aaa.###.ccc", "MIRASIM_OAUTH_TOKEN_UNPARSABLE"},
		{
			// 关键回归：把 access token 当 refresh token 存进来会让 relay 直接 401，
			// 所以这里必须硬拦。
			name:       "access token 被拒绝",
			token:      mirasimTestJWT(t, map[string]any{"email": "u@example.com", "token_type": "access", "exp": 4102444800}),
			wantReason: "MIRASIM_OAUTH_TOKEN_WRONG_TYPE",
		},
		{
			name:       "已过期",
			token:      mirasimTestJWT(t, map[string]any{"email": "u@example.com", "exp": past}),
			wantReason: "MIRASIM_OAUTH_TOKEN_EXPIRED",
		},
		{
			name:       "没有身份声明",
			token:      mirasimTestJWT(t, map[string]any{"plan": "pro", "exp": 4102444800}),
			wantReason: "MIRASIM_OAUTH_TOKEN_NO_IDENTITY",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseMirasimRefreshToken(tc.token)
			mirasimTestAppError(t, err, http.StatusBadRequest, tc.wantReason)
		})
	}
}

func TestMirasimOAuthCompleteWithRefreshTokenSetsProvider(t *testing.T) {
	token := mirasimTestJWT(t, map[string]any{"email": "user@example.com", "exp": 4102444800})
	svc := NewMirasimOAuthService(nil, nil)

	credential, err := svc.CompleteWithRefreshToken("  "+token+"  ", "GitHub")
	require.NoError(t, err)
	// 落库前不得把 token 里的空白带进去。
	require.Equal(t, token, credential.RefreshToken)
	// provider 归一化为小写。
	require.Equal(t, "github", credential.Identity.Provider)

	// provider 非法/缺省时降级为 manual，但不阻断凭据生产。
	credential, err = svc.CompleteWithRefreshToken(token, "not a provider!")
	require.NoError(t, err)
	require.Equal(t, "manual", credential.Identity.Provider)
}

// 回调解析 + 凭据生产必须是纯本地的：这两个方法都不接受 context，也就无法发起任何
// 上游调用（在 nil upstream 上跑通，即是「登录不触发 /auth/refresh 轮换」的证明）。
func TestMirasimOAuthCallbackPathIsOffline(t *testing.T) {
	token := mirasimTestJWT(t, map[string]any{"email": "user@example.com", "exp": 4102444800})
	svc := NewMirasimOAuthService(nil, nil) // nil upstream：一旦联网就会失败/panic
	credential, err := svc.CompleteWithCallbackURL("http://127.0.0.1:9/callback?refresh_token="+token, "google")
	require.NoError(t, err)
	require.Equal(t, token, credential.RefreshToken)
	require.Equal(t, "google", credential.Identity.Provider)
}

func TestMirasimOAuthInputNormalization(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		got, err := mirasimOAuthNormalizeProvider(" GitHub ")
		require.NoError(t, err)
		require.Equal(t, "github", got)

		_, err = mirasimOAuthNormalizeProvider("   ")
		mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_PROVIDER_REQUIRED")

		// provider 会被拼进上游路径，必须挡住路径穿越与注入。
		for _, bad := range []string{"../admin", "github/login", "GITHUB/../x", "gh osp", "github%2f.."} {
			_, err := mirasimOAuthNormalizeProvider(bad)
			mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_PROVIDER_INVALID")
		}
	})

	t.Run("email", func(t *testing.T) {
		got, err := mirasimOAuthNormalizeEmail(" User@Example.COM ")
		require.NoError(t, err)
		require.Equal(t, "user@example.com", got)

		for _, bad := range []string{"", "user", "a b@example.com", "user@exa mple.com"} {
			_, err := mirasimOAuthNormalizeEmail(bad)
			mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_EMAIL_INVALID")
		}
	})
}

func TestMirasimOAuthResolveAuthURL(t *testing.T) {
	svc := NewMirasimOAuthService(nil, nil)

	got, err := svc.resolveAuthURL("")
	require.NoError(t, err)
	require.Equal(t, mirasimDefaultAuthURL, got)
	// auth 地址必须与 relay 默认值体系一致（默认 https://auth.mirasim.ai）。
	require.Equal(t, "https://auth.mirasim.ai", got)

	got, err = svc.resolveAuthURL(" https://auth.example.com/ ")
	require.NoError(t, err)
	require.Equal(t, "https://auth.example.com", got)

	for _, bad := range []string{"auth.mirasim.ai", "ftp://auth.mirasim.ai", "http://"} {
		_, err := svc.resolveAuthURL(bad)
		mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_AUTH_URL_INVALID")
	}
}

func TestMirasimOAuthUpstreamErrorClassification(t *testing.T) {
	cases := []struct {
		status     int
		wantStatus int
		wantReason string
	}{
		{http.StatusBadRequest, http.StatusBadRequest, "MIRASIM_OAUTH_UPSTREAM_REJECTED"},
		{http.StatusUnprocessableEntity, http.StatusBadRequest, "MIRASIM_OAUTH_UPSTREAM_REJECTED"},
		{http.StatusUnauthorized, http.StatusBadRequest, "MIRASIM_OAUTH_UPSTREAM_DENIED"},
		{http.StatusForbidden, http.StatusBadRequest, "MIRASIM_OAUTH_UPSTREAM_DENIED"},
		{http.StatusNotFound, http.StatusBadRequest, "MIRASIM_OAUTH_UPSTREAM_NOT_FOUND"},
		{http.StatusInternalServerError, http.StatusBadGateway, "MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE"},
		{http.StatusBadGateway, http.StatusBadGateway, "MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE"},
	}
	for _, tc := range cases {
		err := mirasimOAuthUpstreamError("verify email code", tc.status, []byte(`{"detail":"bad code"}`))
		mirasimTestAppError(t, err, tc.wantStatus, tc.wantReason)

		// 5xx 视为上游故障，不得把上游细节透给管理端。
		var appErr *infraerrors.ApplicationError
		require.ErrorAs(t, err, &appErr)
		if tc.status >= 500 {
			require.NotContains(t, appErr.Message, "bad code")
		} else {
			require.Contains(t, appErr.Message, "bad code")
		}
	}
}

// OAuth 上游错误信封是 FastAPI 风格 {"detail": "..."}；detail 为空时退回原始 body。
func TestMirasimOAuthUpstreamDetail(t *testing.T) {
	require.Equal(t, "invalid code", mirasimOAuthUpstreamDetail([]byte(`{"detail":"invalid code"}`)))
	require.Equal(t, "boom", mirasimOAuthUpstreamDetail([]byte(`{"error":"boom"}`)))
	require.Equal(t, "oops", mirasimOAuthUpstreamDetail([]byte(`{"message":"oops"}`)))
	require.Equal(t, "", mirasimOAuthUpstreamDetail([]byte(`{"other":"x"}`)))
	require.Equal(t, "", mirasimOAuthUpstreamDetail(nil))
	// 非 JSON 时返回空串（由调用方决定是否用原始 body 兜底）。
	require.Equal(t, "", mirasimOAuthUpstreamDetail([]byte("<html>502</html>")))
}

// ListProviders 的响应兼容性：上游当前是 {"providers":[...]}，也兼容裸数组；
// 非法 ID 与重复项必须被剔除，且请求是 GET 到 /auth/oauth/providers。
func TestMirasimOAuthListProvidersParsing(t *testing.T) {
	bodies := []string{
		`{"providers":["github","google"]}`,
		`["github","google"]`,
		`{"providers":["GitHub"," google ","github","bad provider!","../x"]}`,
	}
	for _, body := range bodies {
		upstream := &mirasimOAuthFakeUpstream{status: http.StatusOK, body: []byte(body)}
		svc := NewMirasimOAuthService(nil, upstream)
		providers, err := svc.ListProviders(context.Background(), "", nil)
		require.NoError(t, err, body)
		require.Equal(t, []string{"github", "google"}, providers, body)
		require.Equal(t, 1, upstream.calls)
		require.Equal(t, http.MethodGet, upstream.method)
		require.Equal(t, mirasimDefaultAuthURL+mirasimOAuthProvidersPath, upstream.url)
	}
}

func TestMirasimOAuthListProvidersEmptyIsGatewayError(t *testing.T) {
	upstream := &mirasimOAuthFakeUpstream{status: http.StatusOK, body: []byte(`{"providers":[]}`)}
	svc := NewMirasimOAuthService(nil, upstream)
	_, err := svc.ListProviders(context.Background(), "", nil)
	mirasimTestAppError(t, err, http.StatusBadGateway, "MIRASIM_OAUTH_PROVIDERS_EMPTY")
}

func TestMirasimOAuthListProvidersUpstreamError(t *testing.T) {
	upstream := &mirasimOAuthFakeUpstream{status: http.StatusInternalServerError, body: []byte(`{"detail":"boom"}`)}
	svc := NewMirasimOAuthService(nil, upstream)
	_, err := svc.ListProviders(context.Background(), "", nil)
	mirasimTestAppError(t, err, http.StatusBadGateway, "MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE")
}

// GenerateAuthURL 不需要网络：只拼链接。redirect_uri 必须是 loopback 占位符，
// 否则上游白名单会直接 400。
func TestMirasimOAuthGenerateAuthURLUsesLoopbackRedirect(t *testing.T) {
	svc := NewMirasimOAuthService(nil, nil)
	result, err := svc.GenerateAuthURL(context.Background(), "GitHub", "", nil)
	require.NoError(t, err)
	require.Equal(t, "github", result.Provider)
	require.Equal(t, mirasimOAuthManualRedirectURI, result.RedirectURI)
	require.Contains(t, result.RedirectURI, "127.0.0.1")
	require.True(t, strings.HasPrefix(result.AuthURL, mirasimDefaultAuthURL+"/auth/oauth/github/login?"), result.AuthURL)
	require.Contains(t, result.AuthURL, "redirect_uri=http%3A%2F%2F127.0.0.1%3A9%2Fcallback")
	require.Contains(t, result.AuthURL, "state="+result.State)
	// state 是本地关联 ID：非空、长度固定、每次不同。
	require.Len(t, result.State, mirasimOAuthStateBytes*2)
	require.Greater(t, result.ExpiresAt, time.Now().Unix())
	require.LessOrEqual(t, result.ExpiresAt, time.Now().Add(mirasimOAuthFlowTTLSeconds*time.Second).Unix())

	other, err := svc.GenerateAuthURL(context.Background(), "github", "", nil)
	require.NoError(t, err)
	require.NotEqual(t, result.State, other.State)

	// provider 非法时必须在生成链接前就失败。
	_, err = svc.GenerateAuthURL(context.Background(), "gh osp", "", nil)
	mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_PROVIDER_INVALID")
}

// credentialFromAuthResponse 的宽容取值：refresh_token / token，以及 data / result 包裹。
func TestMirasimOAuthCredentialFromAuthResponse(t *testing.T) {
	token := mirasimTestJWT(t, map[string]any{"email": "user@example.com", "exp": 4102444800})
	svc := NewMirasimOAuthService(nil, nil)
	bodies := []string{
		`{"refresh_token":"` + token + `"}`,
		`{"token":"` + token + `"}`,
		`{"data":{"refresh_token":"` + token + `"}}`,
		`{"result":{"token":"` + token + `"}}`,
	}
	for _, body := range bodies {
		credential, err := svc.credentialFromAuthResponse([]byte(body), "email")
		require.NoError(t, err, body)
		require.Equal(t, token, credential.RefreshToken, body)
		require.Equal(t, "email", credential.Identity.Provider, body)
	}

	_, err := svc.credentialFromAuthResponse([]byte(`{"ok":true}`), "email")
	mirasimTestAppError(t, err, http.StatusBadGateway, "MIRASIM_OAUTH_VERIFY_NO_TOKEN")

	_, err = svc.credentialFromAuthResponse([]byte(`<html>502</html>`), "email")
	mirasimTestAppError(t, err, http.StatusBadGateway, "MIRASIM_OAUTH_VERIFY_BAD_RESPONSE")
}

// 没有 httpUpstream 时必须返回明确错误，而不是 panic（生产里上游客户端始终注入）。
func TestMirasimOAuthMissingUpstreamFails(t *testing.T) {
	svc := NewMirasimOAuthService(nil, nil)
	_, err := svc.ListProviders(context.Background(), "", nil)
	mirasimTestAppError(t, err, http.StatusServiceUnavailable, "MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE")
}

// 传了 proxyID 但没有 proxyRepo 时必须报错而不是静默直连（否则会绕过代理治理）。
func TestMirasimOAuthProxyRequiresRepository(t *testing.T) {
	svc := NewMirasimOAuthService(nil, &mirasimOAuthFakeUpstream{})
	proxyID := int64(7)
	_, err := svc.ListProviders(context.Background(), "", &proxyID)
	mirasimTestAppError(t, err, http.StatusBadRequest, "MIRASIM_OAUTH_PROXY_NOT_AVAILABLE")
}

// mirasimOAuthFakeUpstream 是 HTTPUpstream 的最小假实现：只记录最后一个请求，
// 不联网。始终返回配置的 status/body。
type mirasimOAuthFakeUpstream struct {
	status int
	body   []byte

	url    string
	method string
	calls  int
}

func (f *mirasimOAuthFakeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	f.calls++
	f.url = req.URL.String()
	f.method = req.Method
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	body := f.body
	if body == nil {
		body = []byte(`{}`)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func (f *mirasimOAuthFakeUpstream) DoWithTLS(
	req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile,
) (*http.Response, error) {
	return f.Do(req, proxyURL, accountID, concurrency)
}
