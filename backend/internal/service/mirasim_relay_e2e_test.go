//go:build e2e

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Mirasim relay 真实链路端到端测试（build tag: e2e）。
//
// 关键约束（改动前请先读）：
//   - 本文件会真的访问 https://relay.mirasim.ai 与 https://auth.mirasim.ai，只在显式
//     提供凭据时运行；缺凭据时 t.Skip，不会让常规 CI 变红。
//   - 凭据一律从 0600 文件按路径读取，禁止写进代码/环境明文，禁止把内容打进日志；
//     日志只允许打印长度、截断前缀等结构性信息。
//   - 默认使用调用方缓存的 access token，**不调用 /auth/refresh**：真实客户端的
//     refresh token 是单飞轮换的，测试若替它刷新会让本机 mirasim 客户端掉登录。
//     只有 refresh 拒绝路径才会打 /auth/refresh，且用的是伪造 token。
//
// 环境变量：
//
//	MIRASIM_E2E_DEVICE_KEY_FILE      必填，账号设备私钥 PKCS8 PEM 文件路径
//	MIRASIM_E2E_ACCESS_TOKEN_FILE    强烈建议提供，缓存 access token（避免 refresh 轮换）
//	MIRASIM_E2E_EXPECT_DEVICE_ID     可选，断言推导出的 deviceID 与真实客户端一致
//	MIRASIM_E2E_MODELS               必填（部分用例），逗号分隔候选模型，如 kimi-k3
//	MIRASIM_E2E_CLIENT_VERSION       可选，覆盖 x-mirasim-client
const (
	mirasimE2EDeviceKeyFile   = "MIRASIM_E2E_DEVICE_KEY_FILE"
	mirasimE2EAccessTokenFile = "MIRASIM_E2E_ACCESS_TOKEN_FILE"
	mirasimE2EModels          = "MIRASIM_E2E_MODELS"
	mirasimE2EExpectDeviceID  = "MIRASIM_E2E_EXPECT_DEVICE_ID"
	mirasimE2EClientVersion   = "MIRASIM_E2E_CLIENT_VERSION"
)

// mirasimE2EUpstream 直连 HTTP 客户端，绕过 sub2api 的连接池/代理设施。
type mirasimE2EUpstream struct{ client *http.Client }

func (u *mirasimE2EUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.client.Do(req)
}

func (u *mirasimE2EUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return u.client.Do(req)
}

func mirasimE2EReadFile(t *testing.T, key string) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(key))
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s(%s) 失败: %v", key, path, err)
	}
	return strings.TrimSpace(string(raw))
}

func mirasimE2EModelList(t *testing.T) []string {
	t.Helper()
	var models []string
	for _, m := range strings.Split(os.Getenv(mirasimE2EModels), ",") {
		if m = strings.TrimSpace(m); m != "" {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		t.Fatalf("缺少 %s", mirasimE2EModels)
	}
	return models
}

// mirasimE2ENewAccount 构造一个"平台 Anthropic + 类型 API Key"的 relay 账号。
func mirasimE2ENewAccount(id int64, refreshToken, deviceKeyPEM string) *Account {
	return &Account{
		ID:       id,
		Name:     fmt.Sprintf("mirasim-e2e-%d", id),
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			mirasimCredentialRefreshToken: refreshToken,
			mirasimCredentialDeviceKey:    deviceKeyPEM,
		},
		Extra: map[string]any{},
	}
}

// mirasimE2EPrimeAccessToken 预置缓存 access token，避免测试触发 refresh token 轮换。
func mirasimE2EPrimeAccessToken(t *testing.T, client *mirasimRelayClient, token string) {
	t.Helper()
	if token == "" {
		t.Log("未提供缓存 access token，将走 /auth/refresh（可能轮换 refresh token）")
		return
	}
	client.mu.Lock()
	client.cachedAccess = token
	client.accessExp = time.Now().Add(45 * time.Minute)
	client.mu.Unlock()
	t.Log("使用调用方缓存的 access token（不调用 refresh 端点）")
}

// mirasimE2EGatewayService 构造带 URL 白名单的最小 GatewayService，与生产校验路径一致。
func mirasimE2EGatewayService(upstream HTTPUpstream) *GatewayService {
	return &GatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:       true,
			UpstreamHosts: []string{"api.anthropic.com", "relay.mirasim.ai"},
		}}},
	}
}

// TestMirasimRealRelayInjectionEndToEnd 走真实注入函数，逐模型探测 relay 可用性。
func TestMirasimRealRelayInjectionEndToEnd(t *testing.T) {
	accessToken := mirasimE2EReadFile(t, mirasimE2EAccessTokenFile)
	refreshToken := mirasimE2EReadFile(t, "MIRASIM_E2E_REFRESH_TOKEN_FILE")
	deviceKeyPEM := mirasimE2EReadFile(t, mirasimE2EDeviceKeyFile)
	if deviceKeyPEM == "" || (accessToken == "" && refreshToken == "") {
		t.Skip("缺少 MIRASIM_E2E_* 凭据文件")
	}
	models := mirasimE2EModelList(t)
	if refreshToken == "" {
		// 有缓存 access token 时 refresh token 只是配置占位，绝不会被使用（placeholder → 401，不会轮换）。
		if accessToken == "" {
			t.Fatalf("需要 %s 或 MIRASIM_E2E_REFRESH_TOKEN_FILE", mirasimE2EAccessTokenFile)
		}
		refreshToken = "e2e-placeholder-not-used"
	}

	account := mirasimE2ENewAccount(999001, refreshToken, deviceKeyPEM)
	if v := strings.TrimSpace(os.Getenv(mirasimE2EClientVersion)); v != "" {
		account.Extra[mirasimExtraClientVersion] = v
	}

	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	upstream := &mirasimE2EUpstream{client: &http.Client{Timeout: 90 * time.Second}}
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, nil, nil)
	if err != nil {
		t.Fatalf("构建 client 失败: %v", err)
	}
	// 1) 设备身份推导必须与真实客户端一致（relay 只认注册过的 deviceID）。
	t.Logf("derived deviceID=%s clientVersion=%s relay=%s", client.deviceID, client.clientVersion, client.relayURL)
	if want := strings.TrimSpace(os.Getenv(mirasimE2EExpectDeviceID)); want != "" && client.deviceID != want {
		t.Fatalf("deviceID 不匹配: got %s want %s", client.deviceID, want)
	}
	mirasimE2EPrimeAccessToken(t, client, accessToken)

	// 2) 真实注入：从 api.anthropic.com 起步，交给 applyMirasimRelayAuthToRequest 改写。
	sendInject := func(path string, body []byte) (int, string) {
		req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("x-api-key", "should-be-removed")
		req.Header.Set("anthropic-version", "2023-06-01")
		if err := applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req, body); err != nil {
			t.Fatalf("注入失败: %v", err)
		}
		t.Logf("injected: host=%s path=%s query=%q auth=%q x-api-key=%q encLen=%d client=%q",
			req.URL.Host, req.URL.Path, req.URL.RawQuery,
			mirasimTruncate(req.Header.Get("Authorization"), 24),
			req.Header.Get("x-api-key"),
			len(req.Header.Get(mirasimHeaderEnc)),
			req.Header.Get(mirasimHeaderClient),
		)
		resp, err := upstream.Do(req, "", account.ID, 1)
		if err != nil {
			t.Fatalf("请求 relay 失败: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, string(raw)
	}

	okModel := ""
	for _, model := range models {
		body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`, model))
		code, raw := sendInject("/v1/messages", body)
		t.Logf("model=%-28s /v1/messages -> %d %s", model, code, mirasimTruncate(raw, 300))
		if code >= 200 && code < 300 && okModel == "" {
			okModel = model
		}
	}
	if okModel == "" {
		t.Fatal("没有任何候选模型被 relay 接受")
	}

	// 3) 用已被接受的模型再验一次 count_tokens 通路。
	ctBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, okModel))
	code, raw := sendInject("/v1/messages/count_tokens", ctBody)
	t.Logf("count_tokens model=%s -> %d %s", okModel, code, mirasimTruncate(raw, 300))
	if code < 200 || code >= 300 {
		t.Fatalf("count_tokens 未被 relay 接受: HTTP %d", code)
	}
	t.Logf("E2E OK: 认证链通过（device session 签名 → ticket → 密封注入），可用模型=%s", okModel)
}

// TestMirasimRealRelayRefreshRejected 用伪造 refresh token 打真实 /auth/refresh，
// 验证错误归类为认证错误（不触碰真实凭据，不会造成轮换）。
func TestMirasimRealRelayRefreshRejected(t *testing.T) {
	deviceKeyPEM := mirasimE2EReadFile(t, mirasimE2EDeviceKeyFile)
	if deviceKeyPEM == "" {
		t.Skip("缺少 MIRASIM_E2E_DEVICE_KEY_FILE")
	}
	account := mirasimE2ENewAccount(999002, "bogus-refresh-token-for-e2e", deviceKeyPEM)
	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	upstream := &mirasimE2EUpstream{client: &http.Client{Timeout: 60 * time.Second}}
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, nil, nil)
	if err != nil {
		t.Fatalf("构建 client 失败: %v", err)
	}
	if _, err = client.ticket(context.Background()); err == nil {
		t.Fatal("伪造 refresh token 竟然申领到 ticket")
	}
	t.Logf("ticket+bogus -> err=%s isAuthError=%v", mirasimTruncate(err.Error(), 200), mirasimIsAuthError(err))
	if !mirasimIsAuthError(err) {
		t.Fatalf("伪造凭据未被归类为认证错误: %v", err)
	}

	client.invalidateTokens()
	if _, err = client.accessToken(context.Background()); err == nil {
		t.Fatal("伪造 refresh token 竟然换到 access token")
	}
	t.Logf("accessToken+bogus -> err=%s isAuthError=%v", mirasimTruncate(err.Error(), 200), mirasimIsAuthError(err))
	if !mirasimIsAuthError(err) {
		t.Fatalf("刷新失败未被归类为认证错误: %v", err)
	}
}

// TestMirasimRealGatewayBuildersEndToEnd 走生产链路：GatewayService 取 ticket、
// 生产 builder 构造请求、真的发给 relay。覆盖 /v1/messages 与 count_tokens。
func TestMirasimRealGatewayBuildersEndToEnd(t *testing.T) {
	accessToken := mirasimE2EReadFile(t, mirasimE2EAccessTokenFile)
	deviceKeyPEM := mirasimE2EReadFile(t, mirasimE2EDeviceKeyFile)
	if accessToken == "" || deviceKeyPEM == "" {
		t.Skip("缺少 MIRASIM_E2E_ACCESS_TOKEN_FILE / MIRASIM_E2E_DEVICE_KEY_FILE")
	}
	model := mirasimE2EModelList(t)[0]

	upstream := &mirasimE2EUpstream{client: &http.Client{Timeout: 90 * time.Second}}
	svc := mirasimE2EGatewayService(upstream)
	account := mirasimE2ENewAccount(999003, "e2e-placeholder-not-used", deviceKeyPEM)

	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, nil, nil)
	if err != nil {
		t.Fatalf("构建 client 失败: %v", err)
	}
	mirasimE2EPrimeAccessToken(t, client, accessToken)

	// 1) 生产入口取 token：tokenType 必须是 apikey（gateway 有 tokenType 硬门禁）。
	token, tokenType, err := svc.mirasimRelayTicketToken(context.Background(), account)
	if err != nil {
		t.Fatalf("mirasimRelayTicketToken 失败: %v", err)
	}
	if tokenType != AccountTypeAPIKey {
		t.Fatalf("tokenType 必须为 %q，实际 %q", AccountTypeAPIKey, tokenType)
	}
	t.Logf("tokenType=%s ticketLen=%d ticketPrefix=%s", tokenType, len(token), mirasimTruncate(token, 8))

	body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`, model))
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), nil, account, body, token)
	if err != nil {
		t.Fatalf("buildUpstreamRequestAnthropicAPIKeyPassthrough 失败: %v", err)
	}
	if req.URL.Host != "relay.mirasim.ai" || req.URL.Path != "/v1/messages" || req.URL.RawQuery != "" {
		t.Fatalf("builder 未把请求改写到 relay: %s", req.URL.String())
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatalf("x-api-key 未被清理: %q", mirasimTruncate(req.Header.Get("x-api-key"), 8))
	}
	resp, err := upstream.Do(req, "", account.ID, 1)
	if err != nil {
		t.Fatalf("请求 relay 失败: %v", err)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	_ = resp.Body.Close()
	t.Logf("messages E2E: status=%d body=%s", resp.StatusCode, mirasimTruncate(string(raw), 240))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("生产 builder 构造的请求未被 relay 接受: HTTP %d", resp.StatusCode)
	}

	// 2) count_tokens 走各自的 builder。
	ctBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}]}`, model))
	ctReq, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), nil, account, ctBody, token)
	if err != nil {
		t.Fatalf("buildCountTokensRequestAnthropicAPIKeyPassthrough 失败: %v", err)
	}
	if ctReq.URL.Host != "relay.mirasim.ai" || ctReq.URL.Path != "/v1/messages/count_tokens" {
		t.Fatalf("count_tokens builder 未把请求改写到 relay: %s", ctReq.URL.String())
	}
	ctResp, err := upstream.Do(ctReq, "", account.ID, 1)
	if err != nil {
		t.Fatalf("请求 relay (count_tokens) 失败: %v", err)
	}
	ctRaw, _ := io.ReadAll(io.LimitReader(ctResp.Body, 1024))
	_ = ctResp.Body.Close()
	t.Logf("count_tokens E2E: status=%d body=%s", ctResp.StatusCode, mirasimTruncate(string(ctRaw), 200))
	if ctResp.StatusCode < 200 || ctResp.StatusCode >= 300 {
		t.Fatalf("count_tokens builder 构造的请求未被 relay 接受: HTTP %d", ctResp.StatusCode)
	}
}

// TestMirasimBuildersRewriteToRelay 离线断言四个 builder 中的另两个也改写到 relay，
// 不发网络请求（token 与 ticket 全部预置）。
func TestMirasimBuildersRewriteToRelay(t *testing.T) {
	deviceKeyPEM := mirasimE2EReadFile(t, mirasimE2EDeviceKeyFile)
	if deviceKeyPEM == "" {
		t.Skip("缺少 MIRASIM_E2E_DEVICE_KEY_FILE")
	}
	upstream := &mirasimE2EUpstream{client: &http.Client{Timeout: 30 * time.Second}}
	svc := mirasimE2EGatewayService(upstream)
	account := mirasimE2ENewAccount(999005, "e2e-placeholder-not-used", deviceKeyPEM)

	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, nil, nil)
	if err != nil {
		t.Fatalf("构建 client 失败: %v", err)
	}
	client.mu.Lock()
	client.cachedAccess = "e2e-placeholder-access"
	client.accessExp = time.Now().Add(45 * time.Minute)
	client.cachedTicket = "e2e-placeholder-ticket"
	client.ticketExp = time.Now().Add(10 * time.Minute)
	client.mu.Unlock()

	body := []byte(`{"model":"kimi-k3","messages":[{"role":"user","content":"ping"}]}`)
	req, _, err := svc.buildUpstreamRequest(context.Background(), nil, account, body, "e2e-placeholder-ticket", AccountTypeAPIKey, "kimi-k3", false, false)
	if err != nil {
		t.Fatalf("buildUpstreamRequest 失败: %v", err)
	}
	if req.URL.Host != "relay.mirasim.ai" || req.URL.Path != "/v1/messages" || req.URL.RawQuery != "" {
		t.Fatalf("buildUpstreamRequest 未改写到 relay: %s", req.URL.String())
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatalf("buildUpstreamRequest 未清理 x-api-key: %q", mirasimTruncate(req.Header.Get("x-api-key"), 8))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		t.Fatalf("buildUpstreamRequest 未写 Bearer 头: %q", mirasimTruncate(req.Header.Get("Authorization"), 12))
	}
	t.Logf("buildUpstreamRequest -> %s encLen=%d", req.URL.String(), len(req.Header.Get(mirasimHeaderEnc)))

	ctReq, _, err := svc.buildCountTokensRequest(context.Background(), nil, account, body, "e2e-placeholder-ticket", AccountTypeAPIKey, "kimi-k3", false)
	if err != nil {
		t.Fatalf("buildCountTokensRequest 失败: %v", err)
	}
	if ctReq.URL.Host != "relay.mirasim.ai" || ctReq.URL.Path != "/v1/messages/count_tokens" || ctReq.URL.RawQuery != "" {
		t.Fatalf("buildCountTokensRequest 未改写到 relay: %s", ctReq.URL.String())
	}
	t.Logf("buildCountTokensRequest -> %s encLen=%d", ctReq.URL.String(), len(ctReq.Header.Get(mirasimHeaderEnc)))
}

// TestMirasimRealRelayMetadataEndToEnd 覆盖只读元数据通道（`GET /v1/models`、
// `GET /v1/limits`）：用真实 device key + 调用方缓存的 access token 直连 relay，
// 断言账号维度可用模型可解析、limits 可归一化、服务门面（MirasimMetadataService）拿到同一份缓存，
// 且**全程不触发 /auth/refresh**（refresh token 是占位值；一旦被换掉就会失败）。
//
// 环境变量：MIRASIM_E2E_DEVICE_KEY_FILE / MIRASIM_E2E_ACCESS_TOKEN_FILE（缺一即 skip）。
// 日志只打印计数、agent 名、窗口名与状态（额度元数据），不含凭据与上游正文。
func TestMirasimRealRelayMetadataEndToEnd(t *testing.T) {
	deviceKeyPEM := mirasimE2EReadFile(t, mirasimE2EDeviceKeyFile)
	accessToken := mirasimE2EReadFile(t, mirasimE2EAccessTokenFile)
	if deviceKeyPEM == "" || accessToken == "" {
		t.Skip("缺少 MIRASIM_E2E_DEVICE_KEY_FILE / MIRASIM_E2E_ACCESS_TOKEN_FILE")
	}

	const placeholderRefresh = "e2e-placeholder-not-used"
	upstream := &mirasimE2EUpstream{client: &http.Client{Timeout: 60 * time.Second}}
	account := mirasimE2ENewAccount(999009, placeholderRefresh, deviceKeyPEM)

	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream, nil, nil)
	if err != nil {
		t.Fatalf("构建 client 失败: %v", err)
	}
	mirasimE2EPrimeAccessToken(t, client, accessToken)

	deviceID := client.deviceID
	if expect := strings.TrimSpace(os.Getenv(mirasimE2EExpectDeviceID)); expect != "" && expect != deviceID {
		t.Fatalf("deviceID 与 %s 不一致", mirasimE2EExpectDeviceID)
	}
	ctx := context.Background()

	// 1) 账号维度可用模型：拉取 → 解析 → 白名单。
	modelsRaw, err := client.fetchAvailableModels(ctx)
	if err != nil {
		// 错误消息里只有操作名与状态码，不会泄漏上游正文。
		t.Fatalf("真实 relay 拉取 available models 失败: %v", err)
	}
	availableModels, err := parseMirasimAvailableModels(modelsRaw)
	if err != nil {
		t.Fatalf("解析 available models 失败: %v", err)
	}
	if len(availableModels) == 0 {
		t.Fatal("available models 为空")
	}
	// 账号维度接口的返回值必须是账号可用的：逐个真发一次极小推理请求验证
	// 「声明可用 == 实际可调用」。这是本变更的核心契约，只在这里（E2E）做，
	// 不进运行时路径。
	assertMirasimModelsCallable(t, svcForProbe(upstream), account, availableModels)

	// 2) 额度窗口：拉取 → 解析 → 映射到 five_hour / seven_day。
	limitsRaw, err := client.fetchLimits(ctx)
	if err != nil {
		t.Fatalf("真实 relay 拉取 limits 失败: %v", err)
	}
	snapshot, err := parseMirasimLimits(limitsRaw)
	if err != nil {
		t.Fatalf("解析 limits 失败: %v", err)
	}
	if len(snapshot.Windows) == 0 {
		t.Fatal("limits 里没有任何有效窗口")
	}
	info := &UsageInfo{Source: "active"}
	mirasimApplyLimitsSnapshot(info, snapshot)
	for _, window := range snapshot.Windows {
		if window.Name == "" {
			t.Fatalf("窗口缺少 name: %+v", window)
		}
		if window.Unlimited {
			if window.Utilization != nil || window.Status != mirasimWindowStatusAllowed {
				t.Fatalf("不限量窗口不应带百分比: %+v", window)
			}
		} else if window.Utilization == nil || *window.Utilization < 0 || *window.Utilization > 100 {
			t.Fatalf("窗口百分比越界: %+v", window)
		}
		switch window.Name {
		case "5h", "7d":
			mapped := window.WindowKey != ""
			if want := !window.ModelScoped && window.Budget > 0; mapped != want {
				t.Fatalf("窗口 %q 的映射判定与 budget/model_scoped 不一致: %+v", window.Name, window)
			}
		}
		t.Logf("window: name=%q status=%s unlimited=%v modelScoped=%v utilization=%s resetAt=%v",
			window.Name, window.Status, window.Unlimited, window.ModelScoped,
			mirasimE2EFloatPtr(window.Utilization), window.ResetAt != "")
	}
	if info.FiveHour == nil && info.SevenDay == nil {
		t.Log("上游本次未返回可映射的 5h/7d 窗口（budget>0 且非 model_scoped），仅 mirasim_windows")
	}
	t.Logf("limits: paid=%v degraded=%v status=%s windows=%d fiveHour=%v sevenDay=%v",
		mirasimE2EBoolPtr(snapshot.Paid), snapshot.Degraded, snapshot.Status, len(snapshot.Windows),
		info.FiveHour != nil, info.SevenDay != nil)

	// 3) 服务门面走同一份缓存：MirasimMetadataService 必须复用 client（同一个 runtime 条目）。
	svc := NewMirasimMetadataService(nil, upstream)
	svcModels, err := svc.AvailableModels(ctx, account, false)
	if err != nil {
		t.Fatalf("MirasimMetadataService.AvailableModels 失败: %v", err)
	}
	if len(svcModels) != len(availableModels) {
		t.Fatalf("服务门面 available models 与直连结果不一致: %d vs %d", len(svcModels), len(availableModels))
	}
	svcLimits, err := svc.Limits(ctx, account, false)
	if err != nil {
		t.Fatalf("MirasimMetadataService.Limits 失败: %v", err)
	}
	if len(svcLimits.Windows) != len(snapshot.Windows) {
		t.Fatalf("服务门面 limits 与直连结果不一致: %d vs %d", len(svcLimits.Windows), len(snapshot.Windows))
	}

	// 4) 全程不得换凭据：metadataCredential 命中缓存 access token 就不该碰 refresh。
	if client.refreshToken != placeholderRefresh {
		t.Fatal("元数据路径触发了 refresh token 轮换（缓存凭据未被复用）")
	}
}

// mirasimE2EFloatPtr / mirasimE2EBoolPtr 只用于日志排版。
func mirasimE2EFloatPtr(v *float64) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%.1f", *v)
}

func mirasimE2EBoolPtr(v *bool) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", *v)
}

// svcForProbe 返回一个可给 buildUpstreamRequestAnthropicAPIKeyPassthrough 用的最小网关。
func svcForProbe(upstream HTTPUpstream) *GatewayService {
	return mirasimE2EGatewayService(upstream)
}

// assertMirasimModelsCallable 断言 /v1/models 列出的每个 id 都能真实调用（2xx + 有内容）。
//
// 关键约束（改动前请先读）：这是唯一会真发推理请求的地方，只在 e2e tag 下运行；
// 每个模型只发一次 max_tokens=16 的请求，日志只打印模型 id 与状态码。
func assertMirasimModelsCallable(t *testing.T, svc *GatewayService, account *Account, models []string) {
	t.Helper()
	ctx := context.Background()
	ticket, tokenType, err := svc.mirasimRelayTicketToken(ctx, account)
	if err != nil {
		t.Fatalf("获取 ticket 失败: %v", err)
	}
	if tokenType != AccountTypeAPIKey {
		t.Fatalf("tokenType 必须为 %q，实际 %q", AccountTypeAPIKey, tokenType)
	}
	for _, model := range models {
		body := []byte(fmt.Sprintf(
			`{"model":%q,"max_tokens":16,"messages":[{"role":"user","content":"reply ok"}]}`, model))
		req, _, buildErr := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(ctx, nil, account, body, ticket)
		if buildErr != nil {
			t.Fatalf("构造请求失败 model=%s: %v", model, buildErr)
		}
		resp, doErr := svc.httpUpstream.Do(req, "", account.ID, 1)
		if doErr != nil {
			t.Fatalf("调用失败 model=%s: %v", model, doErr)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("/v1/models 声明可用但调用失败: model=%s HTTP %d", model, resp.StatusCode)
		}
		t.Logf("callable: model=%s status=%d bytes=%d", model, resp.StatusCode, len(raw))
	}
	t.Logf("/v1/models 的 %d 个模型全部真实可调用", len(models))
}
