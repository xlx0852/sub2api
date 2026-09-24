package service

// Mirasim relay 上游接入：凭据链（refresh token → access token → device ticket）
// + 模型路由请求的签名/密封注入。
//
// 协议原语见 mirasim_protocol.go（mrs-sig-v2 / mrs-seal-v1）。本文件只负责：
//   - 识别哪些账号是 mirasim relay 账号（isMirasimRelayAccount）；
//   - 解析账号上的 relay 配置（base_url / extra 覆盖 / 默认值）；
//   - 维持 access token 与 device ticket 的内存缓存（含提前刷新窗口）；
//   - 把 ticket + 签名头 + 密封 blob 写到发往 relay 的请求上。
//
// 关键约束（改动前请先读）：
//   - 模型路由请求只发 Authorization: Bearer <ticket> + x-mirasim-client +
//     x-mirasim-enc；明文签名头只在 /v1/device/session 端点发送；
//   - 签名与非对称密封的 AAD 都含 path 且不带 query，因此注入时必须先把
//     RawQuery 清空，保证"签的 path"与"发的 path"一致；
//   - tokenType 必须保持 "apikey"（gateway_anthropic_passthrough.go /
//     gateway_count_tokens.go 有硬门禁），mirasim 分支只靠本文件识别。

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// 默认端点与协议参数（与参考实现 internal/mirasim/client.go 一致）。
	mirasimDefaultRelayURL      = "https://relay.mirasim.ai"
	mirasimDefaultAuthURL       = "https://auth.mirasim.ai"
	mirasimDefaultClientVersion = "0.0.322"
	mirasimDefaultSealPubkey    = "HlyNMMeGXryasYLJuYQ/9ksCD4AYVVy1zXKAtJdpJn4="

	// 账号凭据键。
	mirasimCredentialRefreshToken = "mirasim_refresh_token"
	mirasimCredentialDeviceKey    = "mirasim_device_key"

	// 账号 extra 覆盖键。mirasim_relay_enabled 仅作为 UI 记忆/关闭开关：
	// 显式 false 时禁用，其余情况以"凭据是否存在"为准。
	mirasimExtraRelayEnabled  = "mirasim_relay_enabled"
	mirasimExtraRelayURL      = "mirasim_relay_url"
	mirasimExtraAuthURL       = "mirasim_auth_url"
	mirasimExtraClientVersion = "mirasim_client_version"
	mirasimExtraSealPubkey    = "mirasim_seal_pubkey"
	// 端点路径。
	mirasimAccessRefreshPath = "/auth/refresh"
	mirasimDeviceSessionPath = "/v1/device/session"

	// 只读元数据端点头（客户端取证 server.cjs：ePn='/v1/limits'、ngo='x-mirasim-probe'）。
	//
	// 可用模型走**账号维度**的 /v1/models（只返回本账号当前可用的模型）；
	// 全局目录 /v1/model-roster 不描述「本账号能用什么」，已不再用于白名单。
	mirasimAvailableModelsPath = "/v1/models"
	mirasimLimitsPath          = "/v1/limits"
	mirasimHeaderProbe         = "x-mirasim-probe"
	mirasimProbeUsage          = "usage"

	// 元数据调用超时（客户端取证 server.cjs：limits 10000ms=tor）。
	// 刻意不复用 mirasimHTTPTimeout（模型转发 60s）。
	mirasimModelsTimeout = 8 * time.Second
	mirasimLimitsTimeout = 10 * time.Second

	// 元数据缓存 TTL：可用模型对齐客户端 10 分钟（ldo=600000）；limits 取 30s，
	// 保证管理员刷新用量时既不打爆上游、又能看到较新的窗口。
	// 显式「同步上游模型」会传 force=true 绕过可用模型缓存。
	mirasimModelsCacheTTL = 10 * time.Minute
	mirasimLimitsCacheTTL = 30 * time.Second

	mirasimAccessTokenEarlyRefresh = 120 * time.Second
	mirasimTicketEarlyRefresh      = 60 * time.Second
	mirasimDefaultAccessTokenTTL   = 3600 // 秒；响应缺 expires_in 时的兜底
	mirasimDefaultTicketTTL        = 900  // 秒；同上
	mirasimHTTPTimeout             = 60 * time.Second
	mirasimMaxResponseBytes        = 1 << 20
	mirasimPersistTimeout          = 10 * time.Second
)

// isMirasimRelayAccount 是 mirasim relay 账号的唯一权威识别规则。
//
// 形态刻意复用 PlatformAnthropic + AccountTypeAPIKey：现成的 base_url 机制、
// 密钥账号的路由/调度/计费通路全部直接可用，避免新增平台或账号类型带来的
// 全栈足迹（quota schema、前端 registry、调度分池等）。
func isMirasimRelayAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformAnthropic || account.Type != AccountTypeAPIKey {
		return false
	}
	// kill switch：extra 显式置 false 时禁用（凭据保留，便于再次开启）。
	if _, ok := account.Extra[mirasimExtraRelayEnabled]; ok && !account.GetExtraBool(mirasimExtraRelayEnabled) {
		return false
	}
	return strings.TrimSpace(account.GetCredential(mirasimCredentialRefreshToken)) != ""
}

// mirasimRelayConfig 是一次请求所需的 mirasim 配置快照。
type mirasimRelayConfig struct {
	AccountID     int64
	RefreshToken  string
	DeviceKeyPEM  string
	RelayURL      string
	AuthURL       string
	ClientVersion string
	SealPubkey    string
	ProxyURL      string
	Concurrency   int
}

// mirasimRelayConfigForAccount 解析账号上的 mirasim 配置。
//
// relay URL 优先级：credentials.base_url（APIKey 账号的自定义 base URL）→
// extra.mirasim_relay_url → 默认 relay。显式传 base_url 的账号天然把流量指向
// 它自己填的 relay，符合"账号即上游"的既有语义。
func mirasimRelayConfigForAccount(account *Account) (mirasimRelayConfig, error) {
	cfg := mirasimRelayConfig{}
	if account == nil {
		return cfg, errors.New("mirasim relay: account 为空")
	}
	cfg.AccountID = account.ID
	cfg.Concurrency = account.Concurrency
	cfg.RefreshToken = strings.TrimSpace(account.GetCredential(mirasimCredentialRefreshToken))
	if cfg.RefreshToken == "" {
		return cfg, fmt.Errorf("mirasim relay: 账号 %d 缺少 %s", account.ID, mirasimCredentialRefreshToken)
	}
	cfg.DeviceKeyPEM = strings.TrimSpace(account.GetCredential(mirasimCredentialDeviceKey))
	cfg.RelayURL = mirasimResolveEndpoint(
		account.GetCredential("base_url"), account.GetExtraString(mirasimExtraRelayURL), mirasimDefaultRelayURL)
	cfg.AuthURL = mirasimResolveEndpoint("", account.GetExtraString(mirasimExtraAuthURL), mirasimDefaultAuthURL)
	cfg.ClientVersion = strings.TrimSpace(account.GetExtraString(mirasimExtraClientVersion))
	if cfg.ClientVersion == "" {
		cfg.ClientVersion = mirasimDefaultClientVersion
	}
	cfg.SealPubkey = strings.TrimSpace(account.GetExtraString(mirasimExtraSealPubkey))
	if cfg.SealPubkey == "" {
		cfg.SealPubkey = mirasimDefaultSealPubkey
	}
	cfg.ProxyURL = mirasimAccountProxyURL(account)
	for _, endpoint := range []string{cfg.RelayURL, cfg.AuthURL} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return cfg, fmt.Errorf("mirasim relay: 非法端点 %q", endpoint)
		}
	}
	return cfg, nil
}

// mirasimAccountProxyURL 与 gateway_pipeline_claude.go 里 st.proxyURL 的判定完全
// 一致：自定义 base URL（relay 直连）时不套代理，否则带出账号代理。两边必须一致，
// 否则 token 走代理、模型请求直连（或反之）会触发上游风控。
func mirasimAccountProxyURL(account *Account) string {
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			return account.Proxy.URL()
		}
	}
	return ""
}

func mirasimResolveEndpoint(primary, fallback, def string) string {
	for _, candidate := range []string{primary, fallback, def} {
		if trimmed := strings.TrimRight(strings.TrimSpace(candidate), "/"); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimRight(def, "/")
}

// fingerprint 用于运行时缓存失效判定。刻意不含 refresh token：token 轮换由
// 客户端自身与 runtime 的 baseToken 比对处理，避免轮换后把 ticket 缓存整体丢弃。
func (c mirasimRelayConfig) fingerprint() string {
	h := sha256.New()
	for _, part := range []string{c.RelayURL, c.AuthURL, c.ClientVersion, c.SealPubkey, c.DeviceKeyPEM, c.ProxyURL} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// mirasimHTTPError 携带状态码，便于判定 401/403 这类凭据类失败。
type mirasimHTTPError struct {
	Op         string
	StatusCode int
	Body       string
}

func (e *mirasimHTTPError) Error() string {
	return fmt.Sprintf("mirasim relay: %s HTTP %d: %s", e.Op, e.StatusCode, mirasimTruncate(e.Body, 512))
}

func mirasimIsAuthError(err error) bool {
	var httpErr *mirasimHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden
	}
	// 只读元数据链路用不带响应正文的错误类型，同样是 401/403 判定。
	var metaErr *mirasimMetadataHTTPError
	if errors.As(err, &metaErr) {
		return metaErr.StatusCode == http.StatusUnauthorized || metaErr.StatusCode == http.StatusForbidden
	}
	return false
}

func mirasimTruncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

// mirasimRelayClient 持有单个账号的 mirasim 状态（设备身份 + token 缓存）。
type mirasimRelayClient struct {
	accountID     int64
	concurrency   int
	proxyURL      string
	relayURL      string
	authURL       string
	clientVersion string
	sealPubkey    string
	deviceKeyPEM  string
	deviceID      string
	devicePubB64  string
	priv          ed25519.PrivateKey
	upstream      HTTPUpstream

	persistRefresh   func(newToken string)
	persistDeviceKey func(pem string)

	mu           sync.Mutex
	refreshToken string
	cachedAccess string
	accessExp    time.Time
	cachedTicket string
	ticketExp    time.Time
}

func newMirasimRelayClient(
	cfg mirasimRelayConfig,
	upstream HTTPUpstream,
	persistRefresh func(newToken string),
	persistDeviceKey func(pem string),
) (*mirasimRelayClient, error) {
	if upstream == nil {
		return nil, errors.New("mirasim relay: upstream HTTP 客户端未初始化")
	}
	deviceKeyPEM := cfg.DeviceKeyPEM
	var priv ed25519.PrivateKey
	if deviceKeyPEM == "" {
		generated, err := mirasimGenerateDeviceKey()
		if err != nil {
			return nil, err
		}
		priv = generated
		encoded, err := mirasimMarshalPrivateKeyPEM(generated)
		if err != nil {
			return nil, err
		}
		deviceKeyPEM = encoded
		if persistDeviceKey != nil {
			persistDeviceKey(encoded)
		}
	} else {
		parsed, err := mirasimParsePrivateKeyPEM(deviceKeyPEM)
		if err != nil {
			// 设备私钥损坏时重新生成，而不是让账号彻底不可用；新身份会随下次
			// 落盘覆盖旧值。
			slog.Warn("mirasim_relay_device_key_invalid", "account_id", cfg.AccountID, "error", err)
			generated, genErr := mirasimGenerateDeviceKey()
			if genErr != nil {
				return nil, genErr
			}
			priv = generated
			encoded, encErr := mirasimMarshalPrivateKeyPEM(generated)
			if encErr != nil {
				return nil, encErr
			}
			deviceKeyPEM = encoded
			if persistDeviceKey != nil {
				persistDeviceKey(encoded)
			}
		} else {
			priv = parsed
		}
	}
	deviceID, pubB64 := mirasimDeviceIdentity(priv)
	return &mirasimRelayClient{
		accountID:        cfg.AccountID,
		concurrency:      cfg.Concurrency,
		proxyURL:         cfg.ProxyURL,
		relayURL:         cfg.RelayURL,
		authURL:          cfg.AuthURL,
		clientVersion:    cfg.ClientVersion,
		sealPubkey:       cfg.SealPubkey,
		deviceKeyPEM:     deviceKeyPEM,
		deviceID:         deviceID,
		devicePubB64:     pubB64,
		priv:             priv,
		upstream:         upstream,
		persistRefresh:   persistRefresh,
		persistDeviceKey: persistDeviceKey,
		refreshToken:     cfg.RefreshToken,
	}, nil
}

func (c *mirasimRelayClient) currentRefreshToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshToken
}

// invalidateTokens 清空内存中的 access token 与 ticket（上游 401 时调用）。
func (c *mirasimRelayClient) invalidateTokens() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cachedAccess = ""
	c.accessExp = time.Time{}
	c.cachedTicket = ""
	c.ticketExp = time.Time{}
}

func (c *mirasimRelayClient) ticket(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedTicket != "" && time.Now().Before(c.ticketExp.Add(-mirasimTicketEarlyRefresh)) {
		return c.cachedTicket, nil
	}
	accessTok, err := c.accessTokenLocked(ctx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]string{
		"publicKey": c.devicePubB64,
		"deviceId":  c.deviceID,
	})
	if err != nil {
		return "", fmt.Errorf("mirasim relay: session 请求体序列化失败: %w", err)
	}
	path := mirasimDeviceSessionPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.relayURL+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessTok)
	// 明文签名头只在这个端点发送（credential = access token）。
	for k, v := range mirasimSignHeaders(c.priv, c.deviceID, mirasimSignParams{
		Method:        http.MethodPost,
		Path:          path,
		Credential:    accessTok,
		ClientVersion: c.clientVersion,
		Body:          body,
	}, mirasimNowMillis(), "") {
		req.Header.Set(k, v)
	}
	respBody, err := c.doJSON(req, "申领 device ticket")
	if err != nil {
		return "", err
	}
	var sr struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return "", fmt.Errorf("mirasim relay: session 响应解析失败: %w", err)
	}
	if strings.TrimSpace(sr.Ticket) == "" {
		return "", errors.New("mirasim relay: session 响应缺少 ticket")
	}
	expiresIn := sr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = mirasimDefaultTicketTTL
	}
	c.cachedTicket = sr.Ticket
	c.ticketExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return c.cachedTicket, nil
}

func (c *mirasimRelayClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessTokenLocked(ctx)
}

func (c *mirasimRelayClient) accessTokenLocked(ctx context.Context) (string, error) {
	if c.cachedAccess != "" && time.Now().Before(c.accessExp.Add(-mirasimAccessTokenEarlyRefresh)) {
		return c.cachedAccess, nil
	}
	body, err := json.Marshal(map[string]string{"refresh_token": c.refreshToken})
	if err != nil {
		return "", fmt.Errorf("mirasim relay: refresh 请求体序列化失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL+mirasimAccessRefreshPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	respBody, err := c.doJSON(req, "刷新 access token")
	if err != nil {
		return "", err
	}
	var rr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &rr); err != nil {
		return "", fmt.Errorf("mirasim relay: refresh 响应解析失败: %w", err)
	}
	if strings.TrimSpace(rr.AccessToken) == "" {
		return "", errors.New("mirasim relay: refresh 响应缺少 access_token")
	}
	expiresIn := rr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = mirasimDefaultAccessTokenTTL
	}
	c.cachedAccess = rr.AccessToken
	c.accessExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
	if rotated := strings.TrimSpace(rr.RefreshToken); rotated != "" && rotated != c.refreshToken {
		c.refreshToken = rotated
		if c.persistRefresh != nil {
			c.persistRefresh(rotated)
		}
	}
	return c.cachedAccess, nil
}

// doJSON 通过服务现有的 HTTPUpstream 端口发送请求（继承代理、账号并发与
// 出口治理），并返回 2xx 的响应体。
func (c *mirasimRelayClient) doJSON(req *http.Request, op string) ([]byte, error) {
	resp, err := c.upstream.Do(req, c.proxyURL, c.accountID, c.concurrency)
	if err != nil {
		return nil, fmt.Errorf("mirasim relay: %s 请求失败: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mirasimMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("mirasim relay: %s 响应读取失败: %w", op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &mirasimHTTPError{Op: op, StatusCode: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// 只读元数据请求（模型清单 / 额度窗口）
//
// 关键约束（改动前请先读）：
//   - 与模型路由请求不同：请求体为空、只发明文签名头（x-mirasim-device/-ts/
//     -nonce/-sig + 非空时的 x-mirasim-client），**不发** x-mirasim-enc；
//     密封（mrs-seal-v1）只发生在被代理的模型流量上；
//   - credential 顺序：缓存 device ticket → 缓存 access token → 走
//     /v1/device/session 取 ticket；只要缓存可用就绝不触发 refresh token 轮换
//     （轮换会与桌面客户端互踢，见 add-mirasim-relay-upstream 的实测结论）；
//   - 错误消息只带操作名与状态码：上游响应正文不得进入日志或管理端响应体。
// ---------------------------------------------------------------------------

// metadataCredential 返回只读元数据请求使用的凭据，以及是否命中了本地缓存。
//
// 关键约束（改动前请先读）：这里刻意**不**调用 accessToken()/refresh：
// ticket 缓存与 access token 缓存都可用时直接复用；只有两者都空了才走
// ticket(ctx)（冷启动时该路径会 refresh 一次，并把轮换结果落盘）。
func (c *mirasimRelayClient) metadataCredential(ctx context.Context) (string, bool, error) {
	now := time.Now()
	c.mu.Lock()
	if ticket := strings.TrimSpace(c.cachedTicket); ticket != "" && now.Before(c.ticketExp) {
		c.mu.Unlock()
		return ticket, true, nil
	}
	if access := strings.TrimSpace(c.cachedAccess); access != "" && now.Before(c.accessExp) {
		c.mu.Unlock()
		return access, true, nil
	}
	c.mu.Unlock()

	ticket, err := c.ticket(ctx)
	if err != nil {
		return "", false, err
	}
	return ticket, false, nil
}

// metadataGet 发送一次只读元数据请求，返回响应体与状态码。
// 非 2xx 时返回 *mirasimMetadataHTTPError（不含上游正文）。
func (c *mirasimRelayClient) metadataGet(
	ctx context.Context,
	op, path string,
	timeout time.Duration,
	extraHeaders map[string]string,
) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	credential, _, err := c.metadataCredential(ctx)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.relayURL+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("mirasim relay: %s 请求构造失败: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	for name, value := range extraHeaders {
		req.Header.Set(name, value)
	}
	signHeaders := mirasimSignHeaders(c.priv, c.deviceID, mirasimSignParams{
		Method:        http.MethodGet,
		Path:          path,
		Credential:    credential,
		ClientVersion: c.clientVersion,
	}, mirasimNowMillis(), "")
	// 客户端只在 clientVersion 非空时带 x-mirasim-client；这里对齐，避免空值头。
	if strings.TrimSpace(c.clientVersion) == "" {
		delete(signHeaders, mirasimHeaderClient)
	}
	for name, value := range signHeaders {
		req.Header.Set(name, value)
	}

	resp, err := c.upstream.Do(req, c.proxyURL, c.accountID, c.concurrency)
	if err != nil {
		return nil, 0, fmt.Errorf("mirasim relay: %s 请求失败: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mirasimMaxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("mirasim relay: %s 响应读取失败: %w", op, err)
	}
	if int64(len(body)) > mirasimMaxResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("mirasim relay: %s 响应体超过 %d 字节", op, mirasimMaxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &mirasimMetadataHTTPError{Op: op, StatusCode: resp.StatusCode}
	}
	return body, resp.StatusCode, nil
}

// metadataFetch 拉取一份只读元数据，401/403 时失效本地凭据缓存并重试一次。
//
// 关键约束（改动前请先读）：只有 401/403 才重试，且最多一次。500/超时/网络错误
// 一律直接失败——抖动凭据会让一个瞬时上游故障升级成账号级不可用。
func (c *mirasimRelayClient) metadataFetch(
	ctx context.Context,
	op, path string,
	timeout time.Duration,
	extraHeaders map[string]string,
) ([]byte, error) {
	body, _, err := c.metadataGet(ctx, op, path, timeout, extraHeaders)
	if err == nil {
		return body, nil
	}
	if !mirasimIsAuthError(err) {
		return nil, err
	}
	c.invalidateTokens()
	body, _, err = c.metadataGet(ctx, op, path, timeout, extraHeaders)
	if err == nil {
		return body, nil
	}
	return nil, err
}

// fetchAvailableModels 拉取账号维度可用模型原始 JSON（GET /v1/models，空 body + 明文签名头）。
func (c *mirasimRelayClient) fetchAvailableModels(ctx context.Context) ([]byte, error) {
	return c.metadataFetch(ctx, "拉取 available models", mirasimAvailableModelsPath, mirasimModelsTimeout, nil)
}

// fetchLimits 拉取额度窗口原始 JSON（GET /v1/limits，必须带 x-mirasim-probe: usage）。
func (c *mirasimRelayClient) fetchLimits(ctx context.Context) ([]byte, error) {
	return c.metadataFetch(ctx, "拉取 limits", mirasimLimitsPath, mirasimLimitsTimeout, map[string]string{
		mirasimHeaderProbe: mirasimProbeUsage,
	})
}

// mirasimMetadataHTTPError 只保留操作名与状态码。
//
// 关键约束（改动前请先读）：元数据的错误消息会被写进管理端可见的响应体
// （用量降级 error 字段），因此这里**不**携带上游响应正文。
type mirasimMetadataHTTPError struct {
	Op         string
	StatusCode int
}

func (e *mirasimMetadataHTTPError) Error() string {
	return fmt.Sprintf("mirasim relay: %s HTTP %d", e.Op, e.StatusCode)
}

// routeAuthHeaders 为模型路由请求生成鉴权头与密封 blob（credential = ticket）。
func (c *mirasimRelayClient) routeAuthHeaders(ctx context.Context, method, path string, body []byte) (ticket, enc string, err error) {
	ticket, err = c.ticket(ctx)
	if err != nil {
		return "", "", err
	}
	signHeaders := mirasimSignHeaders(c.priv, c.deviceID, mirasimSignParams{
		Method:        method,
		Path:          path,
		Credential:    ticket,
		ClientVersion: c.clientVersion,
		Body:          body,
	}, mirasimNowMillis(), "")
	enc, err = mirasimSealHeaders(c.sealPubkey, method, path, signHeaders)
	if err != nil {
		return "", "", err
	}
	return ticket, enc, nil
}

// applyMirasimRelayAuth 是网关 4 个请求构造器的统一注入点。
//
// 必须在 builder 的最后一步调用（用最终 body、在 ApplyHeaderOverrides 之后），
// 否则头/body 不一致会让上游判签名无效。
func (s *GatewayService) applyMirasimRelayAuth(ctx context.Context, account *Account, req *http.Request, body []byte) error {
	if !isMirasimRelayAccount(account) {
		return nil
	}
	var upstream HTTPUpstream
	var repo AccountRepository
	if s != nil {
		upstream = s.httpUpstream
		repo = s.accountRepo
	}
	return applyMirasimRelayAuthToRequest(ctx, upstream, repo, account, req, body)
}

// applyMirasimRelayAuthToRequest 把 mirasim 鉴权写入 req。
//
// 关键约束（改动前请先读）：函数内部**必须**自己再判一次 isMirasimRelayAccount。
// 本函数会把请求主机改写成 relay 并删掉 x-api-key，如果某个调用点漏判，普通账号的
// 请求（含真实 api key 的 host/头）会被错误改写。宁可在两处重复判断，也不要让
// "忘了判断"变成静默的跨上游串号。
func applyMirasimRelayAuthToRequest(
	ctx context.Context, upstream HTTPUpstream, repo AccountRepository, account *Account, req *http.Request, body []byte,
) error {
	if !isMirasimRelayAccount(account) {
		return nil
	}
	if req == nil {
		return errors.New("mirasim relay: 请求为空")
	}
	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		return err
	}
	// 目标主机强制指向 relay（账号 base_url 可能是空的 anthropic 默认值）。
	parsed, err := url.Parse(cfg.RelayURL)
	if err != nil {
		return fmt.Errorf("mirasim relay: 非法 relay URL %q: %w", cfg.RelayURL, err)
	}
	req.URL.Scheme = parsed.Scheme
	req.URL.Host = parsed.Host
	req.Host = ""
	// 签名与密封 AAD 只覆盖 path：先清 query，保证签的 path 就是发的 path。
	req.URL.RawQuery = ""
	path := req.URL.Path

	persistRefresh, persistDeviceKey := mirasimPersistCallbacks(cfg.AccountID, repo)
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, persistRefresh, persistDeviceKey)
	if err != nil {
		return err
	}
	ticket, enc, err := client.routeAuthHeaders(ctx, req.Method, path, body)
	if err != nil {
		if mirasimIsAuthError(err) {
			invalidateMirasimRelayClientTokens(cfg.AccountID)
		}
		return fmt.Errorf("mirasim relay: 获取签名凭据失败: %w", err)
	}

	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Set("Authorization", "Bearer "+ticket)
	req.Header.Set(mirasimHeaderClient, client.clientVersion)
	req.Header.Set(mirasimHeaderEnc, enc)
	if getHeaderRaw(req.Header, "content-type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return nil
}

func mirasimNowMillis() int64 { return time.Now().UnixMilli() }

// mirasimRelayTicketToken 是 GetAccessToken 的 mirasim 分支：返回 device ticket 并
// 沿用 tokenType "apikey"。
//
// 为什么不能返回空串：网关多处把"凭据为空"当成账号配置错误直接失败（例如
// GetAccessToken 自身的 api_key 判定、上游模型同步的前置校验），而 mirasim 账号
// 本来就没有 api_key。返回 ticket 的好处是：
//   - 非空，满足所有既有前置校验；
//   - 与随后构造器里 applyMirasimRelayAuth 取的是同一份缓存 ticket，不会重复换 token；
//   - 即使某条路径忘了注入（未来新增构造器），实际发出的也是有效凭据而非空头。
func (s *GatewayService) mirasimRelayTicketToken(ctx context.Context, account *Account) (string, string, error) {
	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		return "", "", err
	}
	persistRefresh, persistDeviceKey := mirasimPersistCallbacks(cfg.AccountID, s.accountRepo)
	client, err := mirasimRelayRuntimeAcquire(cfg, s.httpUpstream, persistRefresh, persistDeviceKey)
	if err != nil {
		return "", "", err
	}
	ticket, err := client.ticket(ctx)
	if err != nil {
		if mirasimIsAuthError(err) {
			invalidateMirasimRelayClientTokens(cfg.AccountID)
		}
		return "", "", fmt.Errorf("mirasim relay: 获取 device ticket 失败: %w", err)
	}
	return ticket, AccountTypeAPIKey, nil
}
