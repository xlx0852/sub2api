//go:build unit

package service

// Mirasim relay 注入链路单测：从账号凭据出发，走完 refresh → device session →
// 模型请求注入，并在**客户端侧**重新实现上游的校验逻辑（解封 + 验签），确保注入
// 的头不是"看起来像"，而是真的能被 relay 的验签器接受。
//
// 关键约束（改动前请先读）：
//   - 密封是"封给 relay 静态公钥"的，测试里把 extra.mirasim_seal_pubkey 指向测试
//     自己生成的 X25519 公钥，才能用测试私钥解封；
//   - 断言签名时必须用"注入后真实发出的 path"，一旦有人给 req 加回 query 或改写
//     路径，这里会直接失败。

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

type mirasimRelayUpstreamFake struct {
	mu        sync.Mutex
	requests  []mirasimRecordedRequest
	refreshes int
	sessions  int
}

type mirasimRecordedRequest struct {
	Method  string
	URL     string
	Header  http.Header
	Body    string
	Proxy   string
	AcctID  int64
	Account int
}

func (u *mirasimRelayUpstreamFake) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	rec := mirasimRecordedRequest{
		Method: req.Method, URL: req.URL.String(), Header: req.Header.Clone(),
		Body: string(body), Proxy: proxyURL, AcctID: accountID, Account: concurrency,
	}
	u.mu.Lock()
	u.requests = append(u.requests, rec)
	switch {
	case strings.HasSuffix(req.URL.Path, mirasimAccessRefreshPath):
		u.refreshes++
	case strings.HasSuffix(req.URL.Path, mirasimDeviceSessionPath):
		u.sessions++
	}
	u.mu.Unlock()

	var payload string
	switch {
	case strings.HasSuffix(req.URL.Path, mirasimAccessRefreshPath):
		payload = `{"access_token":"test-access-token","refresh_token":"test-rotated-refresh","expires_in":3600}`
	case strings.HasSuffix(req.URL.Path, mirasimDeviceSessionPath):
		payload = `{"ticket":"test-device-ticket","expires_in":900}`
	default:
		payload = `{}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func (u *mirasimRelayUpstreamFake) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func (u *mirasimRelayUpstreamFake) snapshot() []mirasimRecordedRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]mirasimRecordedRequest, len(u.requests))
	copy(out, u.requests)
	return out
}

// mirasimTestAccount 构造一个 mirasim relay 账号 + 测试自持的 seal 密钥对。
func mirasimTestAccount(t *testing.T, accountID int64, extras map[string]any) (*Account, *ecdh.PrivateKey) {
	t.Helper()
	deviceKey, err := mirasimGenerateDeviceKey()
	require.NoError(t, err)
	pemStr, err := mirasimMarshalPrivateKeyPEM(deviceKey)
	require.NoError(t, err)

	sealPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	sealPubB64 := base64.StdEncoding.EncodeToString(sealPriv.PublicKey().Bytes())

	extra := map[string]any{mirasimExtraSealPubkey: sealPubB64}
	for k, v := range extras {
		extra[k] = v
	}
	account := &Account{
		ID:       accountID,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			mirasimCredentialRefreshToken: "test-refresh-token",
			mirasimCredentialDeviceKey:    pemStr,
		},
		Extra: extra,
	}
	return account, sealPriv
}

func mirasimForgetAccount(accountID int64) {
	rt := mirasimRelayClients
	rt.mu.Lock()
	delete(rt.entries, accountID)
	rt.mu.Unlock()
}

// mirasimOpenSealedHeaders 用测试私钥解封 x-mirasim-enc，返回明文串与字段。
func mirasimOpenSealedHeaders(t *testing.T, sealPriv *ecdh.PrivateKey, enc, method, path string) (string, map[string]string) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	require.NoError(t, err)
	require.Greater(t, len(raw), 32+12, "密封 blob 太短")

	ephPubBytes := raw[:32]
	nonce := raw[32:44]
	ct := raw[44:]

	ephPub, err := ecdh.X25519().NewPublicKey(ephPubBytes)
	require.NoError(t, err)
	shared, err := sealPriv.ECDH(ephPub)
	require.NoError(t, err)

	reader := hkdf.New(sha256.New, shared, ephPubBytes, []byte(mirasimSealInfo))
	key := make([]byte, 32)
	_, err = io.ReadFull(reader, key)
	require.NoError(t, err)

	aead, err := chacha20poly1305.New(key)
	require.NoError(t, err)
	aad := mirasimSealVersion + "\n" + strings.ToUpper(method) + "\n" + path
	plain, err := aead.Open(nil, nonce, ct, []byte(aad))
	require.NoError(t, err, "解封失败：AAD 与实际发出的 path 不一致")

	fields := map[string]string{}
	require.NoError(t, json.Unmarshal(plain, &fields))
	return string(plain), fields
}

func TestMirasimRelayInjectionProducesVerifiableSealedHeaders(t *testing.T) {
	const accountID = int64(940001)
	mirasimForgetAccount(accountID)
	defer mirasimForgetAccount(accountID)

	account, sealPriv := mirasimTestAccount(t, accountID, nil)
	upstream := &mirasimRelayUpstreamFake{}

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":8}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://api.anthropic.com/v1/messages?beta=true", strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("x-api-key", "should-be-removed")
	req.Header.Set("anthropic-version", "2023-06-01")

	require.NoError(t, applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req, body))

	// 1) 目标主机被改写为 relay，且 query 被清掉（签名只覆盖 path）。
	require.Equal(t, mirasimDefaultRelayURL+"/v1/messages", req.URL.String())
	// req.Host 必须被清空：非空 Host 会覆盖 URL.Host，导致实际连回 anthropic 却按 relay 签名。
	require.Empty(t, req.Host)
	require.Empty(t, req.URL.RawQuery)

	// 2) 模型请求只带 ticket 与密封头，绝不能再带 x-api-key。
	require.Equal(t, "Bearer test-device-ticket", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("x-api-key"))
	require.Equal(t, mirasimDefaultClientVersion, req.Header.Get(mirasimHeaderClient))
	require.NotEmpty(t, req.Header.Get(mirasimHeaderEnc))
	// 明文签名头不允许出现在模型请求上（只有 /v1/device/session 才发）。
	require.Empty(t, req.Header.Get(mirasimHeaderDevice))
	require.Empty(t, req.Header.Get(mirasimHeaderSig))
	// 既有头不被破坏。
	require.Equal(t, "2023-06-01", req.Header.Get("anthropic-version"))

	// 3) token 链请求各发生一次，且都是发给 relay / auth 的。
	records := upstream.snapshot()
	require.Len(t, records, 2)
	require.Equal(t, 1, upstream.refreshes)
	require.Equal(t, 1, upstream.sessions)
	require.Equal(t, mirasimDefaultAuthURL+mirasimAccessRefreshPath, records[0].URL)
	require.Equal(t, mirasimDefaultRelayURL+mirasimDeviceSessionPath, records[1].URL)
	// refresh 请求不带签名头（上游实现如此）。
	require.Empty(t, records[0].Header.Get(mirasimHeaderSig))

	// 4) session 请求带的是"明文"签名头，且用 access token 作为 credential 签名。
	pemStr, err := mirasimParsePrivateKeyPEM(account.GetCredential(mirasimCredentialDeviceKey))
	require.NoError(t, err)
	deviceID, _ := mirasimDeviceIdentity(pemStr)
	require.Equal(t, deviceID, records[1].Header.Get(mirasimHeaderDevice))
	require.Equal(t, mirasimDefaultClientVersion, records[1].Header.Get(mirasimHeaderClient))
	require.Equal(t, "Bearer test-access-token", records[1].Header.Get("Authorization"))
	ts, err := strconv.ParseInt(records[1].Header.Get(mirasimHeaderTS), 10, 64)
	require.NoError(t, err)
	sig, err := base64.RawURLEncoding.DecodeString(records[1].Header.Get(mirasimHeaderSig))
	require.NoError(t, err)
	expected := mirasimCanonicalString(http.MethodPost, mirasimDeviceSessionPath, ts,
		records[1].Header.Get(mirasimHeaderNonce), deviceID, mirasimDefaultClientVersion,
		"test-access-token", nil, []byte(records[1].Body))
	require.True(t, ed25519.Verify(pemStr.Public().(ed25519.PublicKey), []byte(expected), sig),
		"session 请求签名无法用设备公钥验证")

	// 5) 解封 x-mirasim-enc：含签名头 + relay 元数据（session/agent/call），键按字典序，
	// 且签名可用设备公钥验证（canonical 必须用同一份元数据重建）。
	plain, fields := mirasimOpenSealedHeaders(t, sealPriv, req.Header.Get(mirasimHeaderEnc), http.MethodPost, "/v1/messages")
	require.Less(t, strings.Index(plain, `"x-mirasim-agent"`), strings.Index(plain, `"x-mirasim-call"`))
	require.Less(t, strings.Index(plain, `"x-mirasim-call"`), strings.Index(plain, `"x-mirasim-device"`))
	require.Less(t, strings.Index(plain, `"x-mirasim-session"`), strings.Index(plain, `"x-mirasim-sig"`))
	require.Len(t, fields, 7)
	require.Equal(t, "claude", fields[mirasimHeaderAgent])
	require.NotEmpty(t, fields[mirasimHeaderSession])
	require.NotEmpty(t, fields[mirasimHeaderCall])
	require.Equal(t, deviceID, fields[mirasimHeaderDevice])
	sealedTS, err := strconv.ParseInt(fields[mirasimHeaderTS], 10, 64)
	require.NoError(t, err)
	sealedSig, err := base64.RawURLEncoding.DecodeString(fields[mirasimHeaderSig])
	require.NoError(t, err)
	sealedMeta := map[string]string{
		mirasimHeaderSession: fields[mirasimHeaderSession],
		mirasimHeaderAgent:   fields[mirasimHeaderAgent],
		mirasimHeaderCall:    fields[mirasimHeaderCall],
	}
	canonical := mirasimCanonicalString(http.MethodPost, "/v1/messages", sealedTS,
		fields[mirasimHeaderNonce], deviceID, mirasimDefaultClientVersion,
		"test-device-ticket", sealedMeta, body)
	require.True(t, ed25519.Verify(pemStr.Public().(ed25519.PublicKey), []byte(canonical), sealedSig),
		"密封头里的签名无法用设备公钥验证")

	// 6) 同一账号第二次注入必须命中缓存（不再打 refresh/session）。
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://api.anthropic.com/v1/messages?beta=true", strings.NewReader(string(body)))
	require.NoError(t, err)
	require.NoError(t, applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req2, body))
	require.Len(t, upstream.snapshot(), 2, "ticket 缓存未生效，重复请求了 token 链")
	require.Equal(t, "Bearer test-device-ticket", req2.Header.Get("Authorization"))
	// 每次注入的 ts/nonce/密文都必须不同，否则上游可重放。
	require.NotEqual(t, req.Header.Get(mirasimHeaderEnc), req2.Header.Get(mirasimHeaderEnc))
}

func TestMirasimRelayTicketTokenKeepsAPIKeyType(t *testing.T) {
	const accountID = int64(940002)
	mirasimForgetAccount(accountID)
	defer mirasimForgetAccount(accountID)

	account, _ := mirasimTestAccount(t, accountID, nil)
	upstream := &mirasimRelayUpstreamFake{}
	svc := &GatewayService{httpUpstream: upstream}

	token, tokenType, err := svc.mirasimRelayTicketToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "test-device-ticket", token)
	// 必须保持 "apikey"：gateway_anthropic_passthrough.go / gateway_count_tokens.go
	// 对 anthropic apikey 通路有硬门禁，改成别的值会被直接拒绝。
	require.Equal(t, AccountTypeAPIKey, tokenType)
	require.Len(t, upstream.snapshot(), 2)

	// 缓存共享：紧接着注入不应再次打 token 链。
	body := []byte(`{"model":"claude-sonnet-4-5"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
	require.NoError(t, err)
	require.NoError(t, applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req, body))
	require.Len(t, upstream.snapshot(), 2)
	require.Equal(t, "Bearer test-device-ticket", req.Header.Get("Authorization"))
}

func TestMirasimRelayInjectionSkipsOtherAccounts(t *testing.T) {
	upstream := &mirasimRelayUpstreamFake{}
	cases := []struct {
		name    string
		account *Account
	}{
		{"普通 anthropic apikey", &Account{ID: 940101, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test"}}},
		{"anthropic oauth", &Account{ID: 940102, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
			Credentials: map[string]any{mirasimCredentialRefreshToken: "rt"}}},
		{"openai apikey 带同名凭据", &Account{ID: 940103, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{mirasimCredentialRefreshToken: "rt"}}},
		{"mirasim 被 extra 显式关闭", &Account{ID: 940104, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
			Credentials: map[string]any{mirasimCredentialRefreshToken: "rt"},
			Extra:       map[string]any{mirasimExtraRelayEnabled: false}}},
		{"缺 refresh token", &Account{ID: 940105, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, isMirasimRelayAccount(tc.account))
			body := []byte(`{"model":"claude-sonnet-4-5"}`)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
			require.NoError(t, err)
			req.Header.Set("x-api-key", "sk-test")
			require.NoError(t, applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, tc.account, req, body))
			// 完全未触碰请求：URL、host、鉴权头保持原样。
			require.Equal(t, "https://api.anthropic.com/v1/messages", req.URL.String())
			require.Equal(t, "sk-test", req.Header.Get("x-api-key"))
			require.Empty(t, req.Header.Get("Authorization"))
			require.Empty(t, req.Header.Get(mirasimHeaderEnc))
		})
	}
	require.Empty(t, upstream.snapshot(), "非 mirasim 账号不应产生 token 链请求")
}

func TestMirasimRelayInjectionRejectsMissingRefreshToken(t *testing.T) {
	account := &Account{ID: 940201, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{mirasimCredentialRefreshToken: "rt"}}
	upstream := &mirasimRelayUpstreamFake{}

	// 缺 refresh token 的账号连注入都不该发生（防串号），config 解析也要报错而不是静默发空凭据。
	account.Credentials = map[string]any{"api_key": "sk-test"}
	_, cfgErr := mirasimRelayConfigForAccount(account)
	require.Error(t, cfgErr)

	// 端点非法时应在发起网络请求前失败。
	account.Credentials = map[string]any{mirasimCredentialRefreshToken: "rt"}
	account.Extra = map[string]any{mirasimExtraRelayURL: "://bad"}
	_, cfgErr = mirasimRelayConfigForAccount(account)
	require.Error(t, cfgErr)
	require.Empty(t, upstream.snapshot())
}

func TestMirasimRelayRotationPersistsNewRefreshToken(t *testing.T) {
	const accountID = int64(940301)
	mirasimForgetAccount(accountID)
	defer mirasimForgetAccount(accountID)

	account, _ := mirasimTestAccount(t, accountID, nil)
	upstream := &mirasimRelayUpstreamFake{}

	type persisted struct {
		key   string
		value string
	}
	ch := make(chan persisted, 4)
	cfg, err := mirasimRelayConfigForAccount(account)
	require.NoError(t, err)
	client, err := mirasimRelayRuntimeAcquire(cfg, upstream,
		func(tok string) { ch <- persisted{mirasimCredentialRefreshToken, tok} },
		func(pem string) { ch <- persisted{mirasimCredentialDeviceKey, pem} })
	require.NoError(t, err)
	_, err = client.ticket(context.Background())
	require.NoError(t, err)

	select {
	case got := <-ch:
		require.Equal(t, mirasimCredentialRefreshToken, got.key)
		require.Equal(t, "test-rotated-refresh", got.value)
	default:
		t.Fatal("上游轮换了 refresh token，但没有回调落盘")
	}
	require.Equal(t, "test-rotated-refresh", client.currentRefreshToken())
}

func TestMirasimRelayConcurrentInjectionUsesSingleTokenChain(t *testing.T) {
	const accountID = int64(940401)
	mirasimForgetAccount(accountID)
	defer mirasimForgetAccount(accountID)

	account, sealPriv := mirasimTestAccount(t, accountID, nil)
	upstream := &mirasimRelayUpstreamFake{}

	// 冷启动并发：多个 goroutine 同时注入，token 链只应发生一次 refresh + 一次 session。
	// 注意：断言/解封都放在 wg.Wait() 之后的主 goroutine 里（t.FailNow 不能在子 goroutine 调）。
	var wg sync.WaitGroup
	encs := make([]string, 8)
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := []byte(`{"model":"claude-sonnet-4-5"}`)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
			if err != nil {
				errs[idx] = err
				return
			}
			if err := applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req, body); err != nil {
				errs[idx] = err
				return
			}
			encs[idx] = req.Header.Get(mirasimHeaderEnc)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, upstream.refreshes, "并发冷启动应只刷新一次 access token")
	require.Equal(t, 1, upstream.sessions, "并发冷启动应只申领一次 ticket")
	seen := make(map[string]struct{}, len(encs))
	sessions := make(map[string]struct{}, len(encs))
	calls := make(map[string]struct{}, len(encs))
	for _, enc := range encs {
		require.NotEmpty(t, enc)
		_, fields := mirasimOpenSealedHeaders(t, sealPriv, enc, http.MethodPost, "/v1/messages")
		// 密封块 = 4 签名头 + relay 元数据（session/agent/call）。
		require.Len(t, fields, 7)
		require.Equal(t, "claude", fields[mirasimHeaderAgent])
		require.NotEmpty(t, fields[mirasimHeaderSession])
		require.NotEmpty(t, fields[mirasimHeaderCall])
		sessions[fields[mirasimHeaderSession]] = struct{}{}
		calls[fields[mirasimHeaderCall]] = struct{}{}
		seen[enc] = struct{}{}
	}
	require.Len(t, seen, 8, "并发请求的密封 blob 必须各不相同（nonce 唯一）")
	// 同一会话（同一账号客户端 + 无客户端会话种子）下 session 必须稳定，call 必须每次不同。
	require.Len(t, sessions, 1, "并发请求应共享同一个 relay session")
	require.Len(t, calls, 8, "每次上游尝试必须带不同的 x-mirasim-call")
}
