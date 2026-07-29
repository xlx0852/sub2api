package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyKimiCodingHeaders(t *testing.T) {
	header := make(http.Header)
	account := &Account{Credentials: map[string]any{"device_id": "device-1", "device_name": "host-1", "device_model": "Linux arm64"}}
	applyKimiCodingHeaders(header, account)
	require.Equal(t, "KimiCLI/1.10.6", header.Get("User-Agent"))
	require.Equal(t, "kimi_cli", header.Get("X-Msh-Platform"))
	require.Equal(t, "1.10.6", header.Get("X-Msh-Version"))
	require.Equal(t, "device-1", header.Get("X-Msh-Device-Id"))
}

func TestKimiCodingTargetAndStreamingUsage(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}
	account := &Account{Platform: PlatformKimi, Credentials: map[string]any{"base_url": "https://api.kimi.com/coding/"}}
	target, err := svc.openAIChatCompletionsTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://api.kimi.com/coding/v1/chat/completions", target)
	require.Equal(t, PlatformKimi, normalizeOpenAICompatiblePlatform(PlatformKimi))

	body, err := ensureOpenAIChatStreamUsage([]byte(`{"model":"kimi-k3","stream":true}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(body, "stream_options.include_usage").Bool())
}

func TestNormalizeKimiUpstreamModel(t *testing.T) {
	tests := map[string]string{
		"k3": "k3", "k3-256k": "k3-256k", "kimi-for-coding": "kimi-for-coding", "kimi-for-coding-highspeed": "kimi-for-coding-highspeed",
		"kimi-k3": "k3", "kimi-k3[1m]": "k3", "kimi-k3(1024)": "k3(1024)", "kimi-k3[1m](high)": "k3(high)",
		"kimi-k2.7-code": "kimi-for-coding", "kimi-k2.7-code-highspeed": "kimi-for-coding-highspeed", "k2.7-code": "kimi-for-coding",
		"k3-highspeed": "k3-highspeed",
	}
	for input, expected := range tests {
		require.Equal(t, expected, normalizeKimiUpstreamModel(input))
	}
}

func TestKimiDefaultModelIDsOnlyExposeOfficialCodingIDs(t *testing.T) {
	require.Equal(t, []string{"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"}, KimiDefaultModelIDs())
	require.NotContains(t, KimiDefaultModelIDs(), "k3-highspeed")
	require.NotContains(t, KimiDefaultModelIDs(), "kimi-k3-highspeed")
}

func TestNormalizeKimiToolMessageLinks(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":""},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]},{"role":"tool","content":"ok"}]}`)
	result, err := normalizeKimiToolMessageLinks(body)
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(result, "messages").Array(), 2)
	require.Equal(t, "Tool calls prepared.", gjson.GetBytes(result, "messages.0.reasoning_content").String())
	require.Equal(t, "call_1", gjson.GetBytes(result, "messages.1.tool_call_id").String())
}

func TestNormalizeKimiToolMessageLinksDoesNotGuessAmbiguousID(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"},{"id":"call_2"}]},{"role":"tool","content":"ok"}]}`)
	result, err := normalizeKimiToolMessageLinks(body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(result, "messages.1.tool_call_id").Exists())
}
