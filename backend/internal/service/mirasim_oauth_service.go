package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Mirasim OAuth 登录接入（把上游「浏览器登录 → 取 refresh token」的流程搬进管理端）。
//
// 关键约束（改动前请先读）：
//  1. 本文件 MUST NOT 调用上游 /auth/refresh。上游 refresh token 是单值轮换的，
//     在这里刷新会把它换掉，导致管理员本地 mirasim 客户端掉登录。登录环节只做
//     本地 JWT 解析（不验签、零上游调用），因此没有轮换风险。
//  2. redirect_uri MUST 使用 loopback 占位符（mirasimOAuthManualRedirectURI）。
//     上游 OAUTH_REDIRECT_ALLOWLIST 只放行 loopback 与 *.mirofish.ai，sub2api 的
//     管理服务不是 loopback，因此它无法自己接收回调；回调由管理员从浏览器地址栏
//     复制回来（见 ExtractMirasimRefreshTokenFromCallbackURL）。实测：把 redirect_uri 换成
//     非 loopback 地址，上游直接返回 400。
//  3. 上游会用自己签的 state 覆盖我们传的 state，sub2api 无法校验它，因此 state
//     只作为本地关联 ID / 调试线索，MUST NOT 被当作 CSRF 安全控制。信任边界是
//     「已认证的管理员 API 调用 + 管理员自己粘贴的回调 URL」。
//  4. 绝不打印 refresh token / access token：日志只记 provider / 长度 / exp 这类
//     结构性字段（沿用仓库既有的凭据脱敏纪律）。
//  5. 本流程只生产凭据，不直接创建账号——凭据回填到账号表单，走既有的
//     POST /admin/accounts 落库路径（含敏感凭据合并与调度器快照同步）。

const (
	mirasimOAuthProvidersPath = "/auth/oauth/providers"
	// mirasimOAuthLoginPathFmt 的 provider 由 mirasimOAuthProviderPattern 校验
	// 后才拼接，不接受任意路径片段。
	mirasimOAuthLoginPathFmt = "/auth/oauth/%s/login"
	mirasimOAuthCodePath     = "/auth/code"
	mirasimOAuthVerifyPath   = "/auth/verify"

	// 上游白名单只接受 loopback；这里固定用一个不监听的端口作为占位回调，
	// 管理员授权后从地址栏复制完整链接即可（链接内含上游下发的 refresh token）。
	mirasimOAuthManualRedirectURI = "http://127.0.0.1:9/callback"

	// 上游 oauthFlowTTL = 10 分钟；这里给管理员留同样长的粘贴窗口。
	mirasimOAuthFlowTTLSeconds = 600

	mirasimOAuthStateBytes       = 16
	mirasimOAuthHTTPTimeout      = 30 * time.Second
	mirasimOAuthMaxResponseBytes = 1 << 20
)

// mirasimOAuthProviderPattern 限制 provider 为安全的路径片段（小写字母、数字、
// 下划线、连字符）。上游当前支持 github / google，但不硬编码，便于上游扩展。
var mirasimOAuthProviderPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// MirasimOAuthIdentity 是从 refresh token JWT 本地解析出的身份信息（不验签）。
// Email 属于 PII：只允许出现在响应体里，不得写日志。
type MirasimOAuthIdentity struct {
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	Plan      string `json:"plan,omitempty"`
	PlanExp   int64  `json:"plan_exp,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

// MirasimOAuthCredential 是一次登录得到的凭据（refresh token）+ 本地解析出的身份。
type MirasimOAuthCredential struct {
	RefreshToken string               `json:"refresh_token"`
	Identity     MirasimOAuthIdentity `json:"identity"`
}

// MirasimOAuthAuthURL 是授权链接生成结果。State 是本地随机值（仅作关联 ID）。
type MirasimOAuthAuthURL struct {
	Provider    string `json:"provider"`
	AuthURL     string `json:"auth_url"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
	ExpiresAt   int64  `json:"expires_at"`
}

// MirasimOAuthEmailStartResult 是「发邮箱验证码」的结果。
type MirasimOAuthEmailStartResult struct {
	Email   string `json:"email"`
	Message string `json:"message,omitempty"`
}

// MirasimOAuthService 实现 mirasim 的授权登录（OAuth 授权码 + 邮箱验证码两条路径），
// 最终都收敛到「拿到 refresh token」这一件事。
type MirasimOAuthService struct {
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
}

func NewMirasimOAuthService(proxyRepo ProxyRepository, httpUpstream HTTPUpstream) *MirasimOAuthService {
	return &MirasimOAuthService{proxyRepo: proxyRepo, httpUpstream: httpUpstream}
}

// ListProviders 代理上游 GET /auth/oauth/providers，返回可用登录方式 ID 列表。
func (s *MirasimOAuthService) ListProviders(ctx context.Context, authURL string, proxyID *int64) ([]string, error) {
	base, err := s.resolveAuthURL(authURL)
	if err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	body, status, err := s.do(ctx, http.MethodGet, base+mirasimOAuthProvidersPath, nil, proxyURL)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, mirasimOAuthUpstreamError("list providers", status, body)
	}
	// 上游当前返回 {"providers":["github","google"]}；同时兼容裸数组形态。
	var wrapped struct {
		Providers []string `json:"providers"`
	}
	providers := []string{}
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Providers) > 0 {
		providers = append(providers, wrapped.Providers...)
	} else {
		var plain []string
		if err := json.Unmarshal(body, &plain); err == nil {
			providers = append(providers, plain...)
		}
	}
	out := make([]string, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, p := range providers {
		id := strings.ToLower(strings.TrimSpace(p))
		if !mirasimOAuthProviderPattern.MatchString(id) {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, infraerrors.New(
			http.StatusBadGateway,
			"MIRASIM_OAUTH_PROVIDERS_EMPTY",
			"上游未返回可用的登录方式，请检查 auth 地址是否正确",
		)
	}
	return out, nil
}

// GenerateAuthURL 生成浏览器授权链接。
//
// 上游流程是 GET {auth}/auth/oauth/{provider}/login?redirect_uri=..&state=..
// 并 307 跳到 GitHub / Google；redirect_uri 必须是 loopback（见文件头约束 2）。
func (s *MirasimOAuthService) GenerateAuthURL(
	ctx context.Context, provider, authURL string, proxyID *int64,
) (*MirasimOAuthAuthURL, error) {
	normalized, err := mirasimOAuthNormalizeProvider(provider)
	if err != nil {
		return nil, err
	}
	base, err := s.resolveAuthURL(authURL)
	if err != nil {
		return nil, err
	}
	state, err := mirasimOAuthRandomHex(mirasimOAuthStateBytes)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "MIRASIM_OAUTH_STATE_FAILED", "生成 state 失败: %v", err)
	}
	endpoint := fmt.Sprintf("%s"+mirasimOAuthLoginPathFmt, base, normalized)
	query := url.Values{}
	query.Set("redirect_uri", mirasimOAuthManualRedirectURI)
	query.Set("state", state)

	slog.Info("mirasim_oauth_auth_url_generated", "provider", normalized)
	return &MirasimOAuthAuthURL{
		Provider:    normalized,
		AuthURL:     endpoint + "?" + query.Encode(),
		State:       state,
		RedirectURI: mirasimOAuthManualRedirectURI,
		ExpiresAt:   time.Now().Add(mirasimOAuthFlowTTLSeconds * time.Second).Unix(),
	}, nil
}

// StartEmailCode 触发上游邮箱验证码（POST /auth/code）。这会真的发一封邮件，
// 因此只在管理员显式点击时调用。
func (s *MirasimOAuthService) StartEmailCode(
	ctx context.Context, email, authURL string, proxyID *int64,
) (*MirasimOAuthEmailStartResult, error) {
	normalized, err := mirasimOAuthNormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	base, err := s.resolveAuthURL(authURL)
	if err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	body, status, err := s.do(ctx, http.MethodPost, base+mirasimOAuthCodePath, map[string]any{"email": normalized}, proxyURL)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, mirasimOAuthUpstreamError("send email code", status, body)
	}
	slog.Info("mirasim_oauth_email_code_requested")
	return &MirasimOAuthEmailStartResult{Email: normalized, Message: mirasimOAuthUpstreamDetail(body)}, nil
}

// CompleteEmailCode 用邮箱 + 验证码换 refresh token（POST /auth/verify）。
func (s *MirasimOAuthService) CompleteEmailCode(
	ctx context.Context, email, code, authURL string, proxyID *int64,
) (*MirasimOAuthCredential, error) {
	normalizedEmail, err := mirasimOAuthNormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_CODE_REQUIRED", "验证码不能为空")
	}
	base, err := s.resolveAuthURL(authURL)
	if err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"email": normalizedEmail, "code": normalizedCode}
	body, status, err := s.do(ctx, http.MethodPost, base+mirasimOAuthVerifyPath, payload, proxyURL)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, mirasimOAuthUpstreamError("verify email code", status, body)
	}
	return s.credentialFromAuthResponse(body, "email")
}

// CompleteWithCallbackURL 从管理员粘贴的回调链接里抽出 refresh token 并解析身份。
// 只接受完整链接或裸 token，不做任何上游调用。
func (s *MirasimOAuthService) CompleteWithCallbackURL(raw, provider string) (*MirasimOAuthCredential, error) {
	token, err := ExtractMirasimRefreshTokenFromCallbackURL(raw)
	if err != nil {
		return nil, err
	}
	return s.CompleteWithRefreshToken(token, provider)
}

// CompleteWithRefreshToken 校验（形态 / token_type / exp，纯本地）并解析 refresh token。
func (s *MirasimOAuthService) CompleteWithRefreshToken(token, provider string) (*MirasimOAuthCredential, error) {
	identity, err := ParseMirasimRefreshToken(token)
	if err != nil {
		return nil, err
	}
	// provider 只是备注字段：非法/缺省时记为 manual，不阻断（凭据本身才是关键）。
	identity.Provider = "manual"
	if normalized, perr := mirasimOAuthNormalizeProvider(provider); perr == nil {
		identity.Provider = normalized
	}
	// 只记结构性字段：绝不记 token 本身。
	slog.Info("mirasim_oauth_credential_parsed",
		"provider", identity.Provider,
		"has_name", identity.Name != "",
		"has_plan", identity.Plan != "",
		"expires_at", identity.ExpiresAt,
	)
	return &MirasimOAuthCredential{
		RefreshToken: strings.TrimSpace(token),
		Identity:     *identity,
	}, nil
}

// ExtractMirasimRefreshTokenFromCallbackURL 从浏览器地址栏内容里抽 refresh token。
//
// 兼容三种形态：完整 URL（?refresh_token= 或 #refresh_token=）、以 ?/# 开头的
// 裸参数串、以及直接粘贴的 token 原文。
func ExtractMirasimRefreshTokenFromCallbackURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_CALLBACK_REQUIRED", "回调链接不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		// 不是完整 URL：可能只复制了 ?refresh_token=... 这段，或者干脆是 token 原文。
		parsed, err = url.Parse("http://placeholder/?" + strings.TrimLeft(trimmed, "?#"))
		if err != nil {
			return "", infraerrors.New(
				http.StatusBadRequest,
				"MIRASIM_OAUTH_CALLBACK_UNPARSABLE",
				"无法解析回调链接，请完整复制浏览器地址栏内容",
			)
		}
	}
	query := parsed.Query()
	if token := strings.TrimSpace(query.Get("refresh_token")); token != "" {
		return token, nil
	}
	if fragment := strings.TrimSpace(parsed.Fragment); fragment != "" {
		if values, err := url.ParseQuery(strings.TrimLeft(fragment, "?")); err == nil {
			if token := strings.TrimSpace(values.Get("refresh_token")); token != "" {
				return token, nil
			}
		}
	}
	// 兜底：粘贴的就是 token 原文。
	if mirasimOAuthLooksLikeJWT(trimmed) {
		return trimmed, nil
	}
	return "", infraerrors.New(
		http.StatusBadRequest,
		"MIRASIM_OAUTH_CALLBACK_NO_TOKEN",
		"回调链接里没有 refresh_token 参数，请完整复制浏览器地址栏内容",
	)
}

// ParseMirasimRefreshToken 纯本地校验并解析 refresh token（不验签、零上游调用）。
//
// 规则与上游 admin.parseRefreshToken 一致：三段 JWT；token_type 存在时必须是
// refresh（否则会把 access token 当 refresh token 存进来）；exp 不能已过期；
// 身份取 email，缺省退回 sub，都没有则视为无法识别账号身份。
func ParseMirasimRefreshToken(token string) (*MirasimOAuthIdentity, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_TOKEN_REQUIRED", "refresh token 不能为空")
	}
	if len(strings.Split(trimmed, ".")) != 3 {
		return nil, infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_TOKEN_MALFORMED",
			"不是合法的 JWT：refresh token 应为三段（header.payload.signature），请确认完整复制",
		)
	}
	claims := mirasimJWTClaims(trimmed)
	if claims == nil {
		return nil, infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_TOKEN_UNPARSABLE",
			"无法解析 JWT payload：请确认粘贴的是 refresh token 原文",
		)
	}
	if tokenType, _ := claims["token_type"].(string); tokenType != "" && tokenType != "refresh" {
		return nil, infraerrors.Newf(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_TOKEN_WRONG_TYPE",
			"这是 %s token，不是 refresh token，请改用 refresh token（有效期 30 天的那种）",
			tokenType,
		)
	}
	identity := &MirasimOAuthIdentity{}
	if exp, ok := mirasimNumericClaim(claims["exp"]); ok {
		identity.ExpiresAt = exp
		if exp > 0 && time.Now().Unix() >= exp {
			return nil, infraerrors.New(
				http.StatusBadRequest,
				"MIRASIM_OAUTH_TOKEN_EXPIRED",
				"refresh token 已过期，请重新登录获取",
			)
		}
	}
	identity.Email, _ = claims["email"].(string)
	if identity.Email == "" {
		identity.Email, _ = claims["sub"].(string)
	}
	if strings.TrimSpace(identity.Email) == "" {
		return nil, infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_TOKEN_NO_IDENTITY",
			"JWT 中没有 email 声明，无法识别账号身份",
		)
	}
	identity.Name, _ = claims["name"].(string)
	identity.Plan, _ = claims["plan"].(string)
	if planExp, ok := mirasimNumericClaim(claims["plan_exp"]); ok {
		identity.PlanExp = planExp
	}
	return identity, nil
}

// credentialFromAuthResponse 从 /auth/verify 的响应体里取 refresh token。
// 上游字段名未在本地留档，因此这里做宽容取值：refresh_token / token。
func (s *MirasimOAuthService) credentialFromAuthResponse(body []byte, provider string) (*MirasimOAuthCredential, error) {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, infraerrors.New(
			http.StatusBadGateway,
			"MIRASIM_OAUTH_VERIFY_BAD_RESPONSE",
			"上游返回的不是合法 JSON，无法取出 refresh token",
		)
	}
	for _, key := range []string{"refresh_token", "token"} {
		if token, ok := envelope[key].(string); ok && strings.TrimSpace(token) != "" {
			return s.CompleteWithRefreshToken(token, provider)
		}
	}
	// 兼容 {data:{...}} / {result:{...}} 包裹。
	for _, wrapper := range []string{"data", "result"} {
		nested, ok := envelope[wrapper].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"refresh_token", "token"} {
			if token, ok := nested[key].(string); ok && strings.TrimSpace(token) != "" {
				return s.CompleteWithRefreshToken(token, provider)
			}
		}
	}
	return nil, infraerrors.New(
		http.StatusBadGateway,
		"MIRASIM_OAUTH_VERIFY_NO_TOKEN",
		"上游响应里没有 refresh token 字段，请改用「粘贴回调链接」方式",
	)
}

// resolveAuthURL 归一化 auth 地址：留空用默认值，必须是带 host 的 http(s) URL。
func (s *MirasimOAuthService) resolveAuthURL(authURL string) (string, error) {
	raw := strings.TrimSpace(authURL)
	if raw == "" {
		raw = mirasimDefaultAuthURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_AUTH_URL_INVALID",
			"auth 地址不是合法的 URL（例如 https://auth.mirasim.ai）",
		)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_AUTH_URL_INVALID",
			"auth 地址只支持 http/https",
		)
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/"), nil
}

func (s *MirasimOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "MIRASIM_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_PROXY_NOT_FOUND", "proxy not found")
	}
	return proxy.URL(), nil
}

// do 通过服务既有的 HTTPUpstream 端口发起请求（继承出站治理与 URL 白名单），
// 返回响应体与状态码。注意：非 2xx 也返回 body，交由调用方归类错误。
func (s *MirasimOAuthService) do(
	ctx context.Context, method, endpoint string, payload any, proxyURL string,
) ([]byte, int, error) {
	if s.httpUpstream == nil {
		return nil, 0, infraerrors.New(
			http.StatusServiceUnavailable,
			"MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE",
			"HTTP 上游客户端未初始化",
		)
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, infraerrors.Newf(http.StatusInternalServerError, "MIRASIM_OAUTH_ENCODE_FAILED", "编码请求体失败: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	reqCtx, cancel := context.WithTimeout(ctx, mirasimOAuthHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, endpoint, body)
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusInternalServerError, "MIRASIM_OAUTH_REQUEST_FAILED", "构造请求失败: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, 0, 0)
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusBadGateway, "MIRASIM_OAUTH_REQUEST_FAILED", "请求上游失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, mirasimOAuthMaxResponseBytes))
	if err != nil {
		return nil, resp.StatusCode, infraerrors.Newf(http.StatusBadGateway, "MIRASIM_OAUTH_READ_FAILED", "读取上游响应失败: %v", err)
	}
	return raw, resp.StatusCode, nil
}

// mirasimOAuthUpstreamError 把上游错误转成管理端可见的错误。
// 只在确有必要时透出上游 detail（非 2xx 的 OAuth 错误信封不含凭据）。
func mirasimOAuthUpstreamError(op string, status int, body []byte) error {
	detail := mirasimOAuthUpstreamDetail(body)
	if detail == "" {
		detail = strings.TrimSpace(mirasimTruncate(string(body), 256))
	}
	code := "MIRASIM_OAUTH_UPSTREAM_ERROR"
	message := fmt.Sprintf("mirasim %s 失败（HTTP %d）", op, status)
	if detail != "" {
		message += ": " + detail
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = "MIRASIM_OAUTH_UPSTREAM_REJECTED"
	case http.StatusUnauthorized, http.StatusForbidden:
		code = "MIRASIM_OAUTH_UPSTREAM_DENIED"
	case http.StatusNotFound:
		code = "MIRASIM_OAUTH_UPSTREAM_NOT_FOUND"
	}
	// 状态码 5xx 视为上游故障，避免把 500 细节直接暴露给前端。
	if status >= 500 {
		code = "MIRASIM_OAUTH_UPSTREAM_UNAVAILABLE"
		message = fmt.Sprintf("mirasim %s 失败：上游暂时不可用（HTTP %d）", op, status)
	}
	return infraerrors.New(statusForMirasimOAuthError(status), code, message)
}

// statusForMirasimOAuthError 决定返回给管理端的状态码：上游 4xx 视为管理端输入
// 问题（400），5xx / 其他视为网关问题（502）。
func statusForMirasimOAuthError(status int) int {
	if status >= 400 && status < 500 {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

// mirasimOAuthUpstreamDetail 取上游错误信封里的 detail / error 文案。
func mirasimOAuthUpstreamDetail(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var envelope struct {
		Detail string `json:"detail"`
		Error  string `json:"error"`
		Msg    string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	for _, candidate := range []string{envelope.Detail, envelope.Error, envelope.Msg} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return mirasimTruncate(trimmed, 256)
		}
	}
	return ""
}

func mirasimOAuthNormalizeProvider(provider string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return "", infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_PROVIDER_REQUIRED",
			"登录方式不能为空（github / google）",
		)
	}
	if !mirasimOAuthProviderPattern.MatchString(normalized) {
		return "", infraerrors.New(
			http.StatusBadRequest,
			"MIRASIM_OAUTH_PROVIDER_INVALID",
			"登录方式非法（只允许小写字母、数字、下划线与连字符）",
		)
	}
	return normalized, nil
}

func mirasimOAuthNormalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || !strings.Contains(normalized, "@") || strings.ContainsAny(normalized, " \t\r\n") {
		return "", infraerrors.New(http.StatusBadRequest, "MIRASIM_OAUTH_EMAIL_INVALID", "请输入合法的邮箱地址")
	}
	return normalized, nil
}

func mirasimOAuthRandomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// mirasimOAuthLooksLikeJWT 判断字符串是否像三段 JWT（仅用于回调解析兜底）。
func mirasimOAuthLooksLikeJWT(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || strings.ContainsAny(part, " \t\r\n/?&") {
			return false
		}
	}
	return true
}

// mirasimNumericClaim 兼容 JSON 数字与字符串形态的数值 claim。
func mirasimNumericClaim(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

// mirasimJWTClaims 解析 JWT 的 payload（不验签）。base64url 两种 padding 都兼容。
func mirasimJWTClaims(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		// 数字用 json.Number 解析的兜底（claims 数值通常是整数）。
		decoder := json.NewDecoder(bytes.NewReader(decoded))
		decoder.UseNumber()
		if err := decoder.Decode(&claims); err != nil {
			return nil
		}
	}
	return claims
}
