//go:build unit

package service

// relay 元数据（session / agent / call / locale / collect）与 ticket 过期字段容错的单测。
//
// 关键约束（改动前请先读）：
//   - 这些头必须**只**出现在密封块里；明文线上只允许 Authorization / x-mirasim-client /
//     x-mirasim-enc，否则等于把客户端指纹又暴露回线上；
//   - agent 必须按**最终上游模型**（映射后）推导，测试用映射后的模型名做断言；
//   - call 每次尝试都变，session 同一会话稳定。

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMirasimRelayAgentMapping(t *testing.T) {
	cases := []struct {
		path string
		body string
		want string
	}{
		{"/v1/messages", `{"model":"deepseek-flash"}`, "dsh"},
		{"/v1/messages", `{"model":"glm-5.3-flash"}`, "zcode"},
		{"/v1/messages", `{"model":"kimi-k3"}`, "kimi"},
		{"/v1/messages", `{"model":"claude-sonnet-5"}`, "claude"},
		{"/v1/messages", `{}`, "claude"},
		{"/v1/responses", `{"model":"gpt-5.6-sol"}`, "codex"},
		{"/v1/alpha/search", `{}`, "codex"},
		{"/v1/images/generations", `{}`, "codex"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, mirasimRelayAgent(tc.path, []byte(tc.body)), "path=%s body=%s", tc.path, tc.body)
	}
}

func TestMirasimRelaySessionStableAndSeedDerived(t *testing.T) {
	c := &mirasimRelayClient{accountID: 4242}

	// 无 metadata.user_id：回退到客户端级稳定随机值，且多次调用不变。
	first := c.relaySessionID([]byte(`{"model":"deepseek-flash"}`))
	second := c.relaySessionID([]byte(`{"model":"deepseek-flash"}`))
	require.Equal(t, first, second, "同一客户端 session 必须稳定")
	require.True(t, strings.HasPrefix(first, "mirasim_"))

	// 有 metadata.user_id：按 accountID 派生；同种子稳定、不同种子不同。
	seedA := c.relaySessionID([]byte(`{"metadata":{"user_id":"user-aaa"}}`))
	seedA2 := c.relaySessionID([]byte(`{"metadata":{"user_id":"user-aaa"}}`))
	seedB := c.relaySessionID([]byte(`{"metadata":{"user_id":"user-bbb"}}`))
	require.Equal(t, seedA, seedA2, "同一客户端会话种子必须得到同一 session")
	require.NotEqual(t, seedA, seedB, "不同会话种子必须得到不同 session")
	require.Len(t, strings.TrimPrefix(seedA, "mirasim_"), 32, "派生 session 长度必须为 32 位十六进制")

	// 账号维度隔离：同一种子在不同账号下不得得到同一个 session。
	other := &mirasimRelayClient{accountID: 4243}
	require.NotEqual(t, seedA, other.relaySessionID([]byte(`{"metadata":{"user_id":"user-aaa"}}`)))
}

func TestMirasimRelayMetadataIncludesOptionalHeadersOnlyWhenConfigured(t *testing.T) {
	base := &mirasimRelayClient{accountID: 1}
	meta := base.relayMetadata("/v1/messages", []byte(`{"model":"deepseek-flash"}`))
	require.Equal(t, "dsh", meta[mirasimHeaderAgent])
	require.NotEmpty(t, meta[mirasimHeaderCall])
	require.NotEmpty(t, meta[mirasimHeaderSession])
	require.NotContains(t, meta, mirasimHeaderLocale)
	require.NotContains(t, meta, mirasimHeaderCollect)

	withOptions := &mirasimRelayClient{accountID: 1, locale: "zh-CN", collectOff: true}
	meta2 := withOptions.relayMetadata("/v1/messages", []byte(`{"model":"deepseek-flash"}`))
	require.Equal(t, "zh-CN", meta2[mirasimHeaderLocale])
	require.Equal(t, mirasimCollectOffStr, meta2[mirasimHeaderCollect])
}

func TestMirasimRelayInjectionSealsMetadataAndKeepsWireClean(t *testing.T) {
	const accountID = int64(940101)
	mirasimForgetAccount(accountID)
	defer mirasimForgetAccount(accountID)

	account, sealPriv := mirasimTestAccount(t, accountID, map[string]any{
		mirasimExtraLocale:  "zh-CN",
		mirasimExtraCollect: false,
	})
	upstream := &mirasimRelayUpstreamFake{}
	body := []byte(`{"model":"deepseek-flash","max_tokens":8}`)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
	require.NoError(t, err)
	require.NoError(t, applyMirasimRelayAuthToRequest(context.Background(), upstream, nil, account, req, body, mirasimRelayClients))

	// 明文线上只允许这三个头；元数据必须全部在密封块里。
	require.NotEmpty(t, req.Header.Get(mirasimHeaderEnc))
	for _, leaked := range []string{
		mirasimHeaderSession, mirasimHeaderAgent, mirasimHeaderCall,
		mirasimHeaderLocale, mirasimHeaderCollect,
	} {
		require.Empty(t, req.Header.Get(leaked), "元数据头 %s 不得出现在明文线上", leaked)
	}

	_, fields := mirasimOpenSealedHeaders(t, sealPriv, req.Header.Get(mirasimHeaderEnc), http.MethodPost, "/v1/messages")
	require.Len(t, fields, 9, "4 签名头 + session/agent/call/locale/collect")
	require.Equal(t, "dsh", fields[mirasimHeaderAgent])
	require.Equal(t, "zh-CN", fields[mirasimHeaderLocale])
	require.Equal(t, mirasimCollectOffStr, fields[mirasimHeaderCollect])
	require.NotEmpty(t, fields[mirasimHeaderSession])
	require.NotEmpty(t, fields[mirasimHeaderCall])
}

func TestMirasimRelayTicketExpiryFieldTolerance(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"camelCase expiresIn", `{"ticket":"t","expiresIn":600}`, 590 * time.Second, 605 * time.Second},
		{"camelCase expiresAt", `{"ticket":"t","expiresAt":` + futureMillis(600) + `}`, 590 * time.Second, 605 * time.Second},
		{"snake_case expires_in", `{"ticket":"t","expires_in":300}`, 290 * time.Second, 305 * time.Second},
		{"no expiry", `{"ticket":"t"}`, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := &mirasimTicketPayloadUpstream{payload: tc.payload}
			priv, err := mirasimGenerateDeviceKey()
			require.NoError(t, err)
			deviceID, devicePub := mirasimDeviceIdentity(priv)
			client := &mirasimRelayClient{
				accountID:     1,
				relayURL:      "https://relay.test",
				authURL:       "https://auth.test",
				upstream:      up,
				refreshToken:  "rt",
				priv:          priv,
				deviceID:      deviceID,
				devicePubB64:  devicePub,
				clientVersion: "0.0.354",
			}
			_, err = client.ticket(context.Background())
			require.NoError(t, err)
			remaining := time.Until(client.ticketExp)
			if tc.wantMax == 0 {
				// 无可用过期字段：必须回落到默认 TTL。
				require.InDelta(t, mirasimDefaultTicketTTL, remaining.Seconds(), 5)
				return
			}
			require.Greater(t, remaining, tc.wantMin)
			require.Less(t, remaining, tc.wantMax)
		})
	}
}

func futureMillis(seconds int) string {
	return strconv.FormatInt(time.Now().Add(time.Duration(seconds)*time.Second).UnixMilli(), 10)
}

// mirasimTicketPayloadUpstream 只回答 refresh 与 device-session 两个端点，用于
// 校验 ticket 过期字段解析。
type mirasimTicketPayloadUpstream struct {
	payload string
}

func (u *mirasimTicketPayloadUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	payload := u.payload
	if strings.HasSuffix(req.URL.Path, mirasimAccessRefreshPath) {
		payload = `{"access_token":"at","expires_in":3600}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func (u *mirasimTicketPayloadUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}
