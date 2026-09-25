package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func gzipTestConfig(minBytes int64, hosts ...string) *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.UpstreamGzipMinBytes = minBytes
	cfg.Gateway.UpstreamGzipHosts = hosts
	return cfg
}

func zstdTestConfig(minBytes int64, hosts ...string) *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.UpstreamGzipMinBytes = minBytes
	cfg.Gateway.UpstreamZstdHosts = hosts
	return cfg
}

func gzipTestAccount(baseURL string) *Account {
	return &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": baseURL},
	}
}

func TestMaybeGzipOpenAIUpstreamBodyCompressesWhitelistedHost(t *testing.T) {
	body := bytes.Repeat([]byte("compressed-please-"), 4096)
	req, _ := http.NewRequest(http.MethodPost, "https://o10.top/v1/responses", bytes.NewReader(body))
	account := gzipTestAccount("https://o10.top")
	cfg := gzipTestConfig(1024, "o10.top")

	if !maybeCompressOpenAIUpstreamBody(req, body, account, cfg) {
		t.Fatal("expected compression to apply")
	}
	if got := req.Header.Get(openAIRequestCompressionHeader); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if req.ContentLength != int64(len(mustReadAll(t, req))) {
		t.Fatalf("ContentLength=%d does not match compressed body", req.ContentLength)
	}
	reader, err := gzip.NewReader(bytes.NewReader(mustReadAll(t, req)))
	if err != nil {
		t.Fatalf("compressed body is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("gzip decode failed: %v", err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatal("round-trip mismatch")
	}
}

func TestMaybeGzipOpenAIUpstreamBodySkipsWhen(t *testing.T) {
	large := bytes.Repeat([]byte("x"), 4096)
	cases := []struct {
		name    string
		account *Account
		cfg     *config.Config
	}{
		{"host not whitelisted", gzipTestAccount("https://api.openai.com"), gzipTestConfig(1024, "o10.top")},
		{"compression disabled", gzipTestAccount("https://o10.top"), gzipTestConfig(0, "o10.top")},
		{"empty allowlist", gzipTestAccount("https://o10.top"), gzipTestConfig(1024)},
		{"oauth account uses zstd path", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://o10.top"}}, gzipTestConfig(1024, "o10.top")},
		{"external bridge upstream", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}, Credentials: map[string]any{"base_url": "https://o10.top"}}, gzipTestConfig(1024, "o10.top")},
		{"non openai platform", &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://o10.top"}}, gzipTestConfig(1024, "o10.top")},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(http.MethodPost, "https://o10.top/v1/responses", bytes.NewReader(large))
		if maybeCompressOpenAIUpstreamBody(req, large, tc.account, tc.cfg) {
			t.Fatalf("%s: compression must be skipped", tc.name)
		}
		if req.Header.Get(openAIRequestCompressionHeader) != "" {
			t.Fatalf("%s: Content-Encoding must stay unset", tc.name)
		}
	}
}

func TestMaybeGzipOpenAIUpstreamBodyRespectsThreshold(t *testing.T) {
	small := bytes.Repeat([]byte("x"), 512)
	req, _ := http.NewRequest(http.MethodPost, "https://o10.top/v1/responses", bytes.NewReader(small))
	if maybeCompressOpenAIUpstreamBody(req, small, gzipTestAccount("https://o10.top"), gzipTestConfig(1024, "o10.top")) {
		t.Fatal("body below threshold must not be compressed")
	}
}

func TestUpstreamGzipHostAllowed(t *testing.T) {
	allowed := []string{"o10.top", "api.openai.com"}
	cases := map[string]bool{
		"https://o10.top":             true,
		"https://api.o10.top/v1":      true,
		"https://O10.TOP":             true,
		"https://api.openai.com":      true,
		"https://evil-o10.top.attack": false,
		"":                            false,
		"https://other.example.com":   false,
	}
	for base, want := range cases {
		if got := upstreamHostAllowed(base, allowed); got != want {
			t.Fatalf("upstreamHostAllowed(%q) = %v, want %v", base, got, want)
		}
	}
}

func mustReadAll(t *testing.T, req *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	// 复原 body，便于后续再次读取（GetBody 也应可用）
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

// TestOpenAIGatewayService_APIKeyUpstreamGzipsLargeBody 覆盖真实转发路径：
// APIKey 账号 + 白名单上游 + 超过阈值 → 上游收到 gzip 请求体，且解压后与原文一致。
func TestOpenAIGatewayService_APIKeyUpstreamGzipsLargeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 2MB 请求体（远高于 1KiB 阈值，压缩收益明显）
	payload := strings.Repeat("gzip-upstream-body-", 110_000)
	originalBody := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"text","text":"` + payload + `"}]}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(originalBody))
	c.Request.Header.Set("Content-Type", "application/json")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","usage":{"input_tokens":11,"output_tokens":22}}`)),
	}
	upstream := &httpUpstreamRecorder{resp: resp}

	cfg := &config.Config{}
	cfg.Gateway.UpstreamGzipMinBytes = 1024
	cfg.Gateway.UpstreamGzipHosts = []string{"o10.top"}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}

	account := &Account{
		ID:             89,
		Name:           "relay",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		Concurrency:    4,
		Credentials:    map[string]any{"api_key": "relay-key", "base_url": "https://o10.top"},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	_, err := svc.Forward(context.Background(), c, account, originalBody)
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "gzip", upstream.lastReq.Header.Get(openAIRequestCompressionHeader),
		"大请求体应带上 Content-Encoding: gzip")
	require.Less(t, len(upstream.lastBodyRaw), len(originalBody)/2,
		"压缩后字节数应显著小于原文")
	reader, gzErr := gzip.NewReader(bytes.NewReader(upstream.lastBodyRaw))
	require.NoError(t, gzErr)
	decoded, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	require.Contains(t, string(decoded), "gzip-upstream-body-", "解压后应还原出原始载荷")
	require.Contains(t, string(upstream.lastBody), "gzip-upstream-body-", "recorder 视角（上游解压后）同样可见载荷")
}

func TestMaybeCompressOpenAIUpstreamBodyPrefersZstd(t *testing.T) {
	body := bytes.Repeat([]byte("compressed-please-"), 8192)
	req, _ := http.NewRequest(http.MethodPost, "https://o10.top/v1/responses", bytes.NewReader(body))
	cfg := zstdTestConfig(1024, "o10.top")
	cfg.Gateway.UpstreamGzipHosts = []string{"o10.top"}

	if !maybeCompressOpenAIUpstreamBody(req, body, gzipTestAccount("https://o10.top"), cfg) {
		t.Fatal("expected compression to apply")
	}
	if got := req.Header.Get(openAIRequestCompressionHeader); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd when host is in zstd allowlist", got)
	}
	compressed := mustReadAll(t, req)
	if len(compressed) >= len(body)/2 {
		t.Fatalf("zstd should shrink payload, got %d of %d", len(compressed), len(body))
	}
}

func TestMaybeCompressOpenAIUpstreamBodyFallsBackToGzip(t *testing.T) {
	body := bytes.Repeat([]byte("compressed-please-"), 8192)
	req, _ := http.NewRequest(http.MethodPost, "https://gzip-only.example.com/v1/responses", bytes.NewReader(body))
	cfg := gzipTestConfig(1024, "gzip-only.example.com")

	if !maybeCompressOpenAIUpstreamBody(req, body, gzipTestAccount("https://gzip-only.example.com"), cfg) {
		t.Fatal("expected gzip fallback to apply")
	}
	if got := req.Header.Get(openAIRequestCompressionHeader); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
}
