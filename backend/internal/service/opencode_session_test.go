package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOfficialOpenCodeHost(t *testing.T) {
	t.Parallel()
	require.True(t, isOfficialOpenCodeHost("https://opencode.ai/zen/go/v1/responses"))
	require.True(t, isOfficialOpenCodeHost("https://opencode.ai/anything"))
	// 只比主机名：伪装的子域必须被排除
	require.False(t, isOfficialOpenCodeHost("https://opencode.ai.evil.example/zen/go/v1"))
	require.False(t, isOfficialOpenCodeHost("http://opencode.ai/zen/go/v1"))
	require.False(t, isOfficialOpenCodeHost(""))
}

// OpenCode Go 要求：官方主机必定要带 session 头；非官方主机不能被塞进去。
func TestApplyOpenCodeUpstreamHeadersOnOfficialHost(t *testing.T) {
	t.Parallel()
	acc := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")

	h := http.Header{}
	applyOpenCodeUpstreamHeaders(h, acc, "https://opencode.ai/zen/go/v1/responses", "")

	require.NotEmpty(t, h.Get(openCodeSessionHeader), "官方主机必须带上 x-opencode-session")
	require.Equal(t, openCodeUpstreamUserAgent, h.Get("user-agent"),
		"UA 必须是 opencode 客户端形态，否则 Cloudflare 1010 会拦成 403")
	require.NotContains(t, h.Get("user-agent"), "codex")
}

func TestApplyOpenCodeUpstreamHeadersSkipsOtherHosts(t *testing.T) {
	t.Parallel()
	acc := openCodeGoTestAccount("https://relay.example.com/v1") // 非 OpenCode 账号
	for _, target := range []string{
		"https://relay.example.com/v1/responses",
		"https://opencode.ai.evil.example/zen/go/v1/responses",
		"https://api.openai.com/v1/responses",
	} {
		h := http.Header{}
		h.Set("user-agent", "codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)")
		applyOpenCodeUpstreamHeaders(h, acc, target, "")
		require.Empty(t, h.Get(openCodeSessionHeader), target)
		require.Contains(t, h.Get("user-agent"), "codex_exec", "非官方主机不得改写 UA：%s", target)
	}
}

// seed 相同 → 值相同。这是硬要求：上游用它做 prompt cache 路由，每轮换新值会持续 miss。
func TestOpenCodeSessionValueIsStableForTheSameSeed(t *testing.T) {
	t.Parallel()
	a := openCodeSessionValue("client-session-abc")
	b := openCodeSessionValue("client-session-abc")
	c := openCodeSessionValue("client-session-xyz")
	require.Equal(t, "client-session-abc", a, "合法标识原样使用")
	require.Equal(t, a, b, "同种子必须同值")
	require.NotEqual(t, a, c, "不同会话应不同值")
	// 无种子时才退一次性 UUID
	require.NotEqual(t, openCodeSessionValue(""), openCodeSessionValue(""))
}

// 客户端自带且合法 → 原样沿用；非法（超长/含奇怪字符）→ 不反射给上游，改用派生值。
func TestApplyOpenCodeUpstreamHeadersPrefersSanitizedClientValue(t *testing.T) {
	t.Parallel()
	acc := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")

	sent := http.Header{}
	sent.Set(openCodeSessionHeader, "client-provided-session-1")
	applyOpenCodeUpstreamHeaders(sent, acc, "https://opencode.ai/zen/go/v1/responses", "seed-ignored")
	require.Equal(t, "client-provided-session-1", sent.Get(openCodeSessionHeader))

	bad := http.Header{}
	bad.Set(openCodeSessionHeader, "bad value with spaces and \n newline")
	applyOpenCodeUpstreamHeaders(bad, acc, "https://opencode.ai/zen/go/v1/responses", "seed-used")
	require.NotContains(t, bad.Get(openCodeSessionHeader), " ")
	require.Equal(t, openCodeSessionValue("seed-used"), bad.Get(openCodeSessionHeader))

	// 既没有合法自带值、也没有种子 → 仍要有值（否则上游 MissingSessionID）
	bare := http.Header{}
	applyOpenCodeUpstreamHeaders(bare, acc, "https://opencode.ai/zen/go/v1/responses", "")
	require.NotEmpty(t, bare.Get(openCodeSessionHeader))
}

func TestSanitizeOpenCodeSessionID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a-b_c.d:e", sanitizeOpenCodeSessionID(" a-b_c.d:e "))
	require.Empty(t, sanitizeOpenCodeSessionID(""))
	require.Empty(t, sanitizeOpenCodeSessionID("has space"))
	require.Empty(t, sanitizeOpenCodeSessionID("has\nnewline"))
	require.Empty(t, sanitizeOpenCodeSessionID(string(make([]byte, 300))))
}

func TestOpenCodeSessionSeedPrefersStableClientIdentifiers(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("session-id", "01a0c48d-5710-7c03-9065-ced0ad8c374f")
	require.Equal(t, "01a0c48d-5710-7c03-9065-ced0ad8c374f", openCodeSessionSeedForRequest(h, nil))

	body := []byte(`{"model":"deepseek-flash","prompt_cache_key":"pc-key-1"}`)
	require.Equal(t, "pc-key-1", openCodeSessionSeedForRequest(http.Header{}, body))
	require.Empty(t, openCodeSessionSeedForRequest(http.Header{}, []byte(`{"model":"x"}`)))
}
