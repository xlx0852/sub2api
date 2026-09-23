package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openCodeGoTestAccount(baseURL string) *Account {
	return &Account{
		ID: 9001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Extra:       map[string]any{},
		Credentials: map[string]any{"api_key": "sk-opencode-go-test-key", "base_url": baseURL},
	}
}

// 唯一决定"是否可以向 opencode.ai 发送这个 key"的判断，必须精确。
func TestIsOpenCodeGoUsageURLAcceptsOnlyTheOfficialEndpoint(t *testing.T) {
	t.Parallel()
	ok := []string{
		"https://opencode.ai/zen/go/v1",
		"https://opencode.ai/zen/go/v1/",
		"https://opencode.ai/zen/go/v1/usage",
	}
	for _, raw := range ok {
		require.True(t, isOpenCodeGoUsageURL(raw), raw)
	}
	bad := []string{
		"", "http://opencode.ai/zen/go/v1", // 非 https
		"https://opencode.ai/zen/go",                 // 缺 /v1
		"https://opencode.ai/zen",                    // 缺 /go/v1
		"https://opencode.ai/zen/go/v1x",             // 仅前缀相同
		"https://opencode.ai.evil.example/zen/go/v1", // 前缀伪装：会把 key 送到别处
		"https://evil.example/zen/go/v1",
		"https://opencode.ai/zen/go/v1/../other",
	}
	for _, raw := range bad {
		require.False(t, isOpenCodeGoUsageURL(raw), raw)
	}
}

func TestIsOpenCodeGoAccountRequiresApiKeyAndOfficialBaseURL(t *testing.T) {
	t.Parallel()
	require.True(t, IsOpenCodeGoAccount(openCodeGoTestAccount("https://opencode.ai/zen/go/v1")))
	// 默认 OpenAI 官方地址不是 OpenCode
	require.False(t, IsOpenCodeGoAccount(openCodeGoTestAccount("")))
	// OAuth 账号不适用（用量端点是给 API key 的）
	oauth := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")
	oauth.Type = AccountTypeOAuth
	require.False(t, IsOpenCodeGoAccount(oauth))
	// 别的中继地址不适用
	require.False(t, IsOpenCodeGoAccount(openCodeGoTestAccount("https://relay.example.com/v1")))
	require.False(t, IsOpenCodeGoAccount(nil))
}

// 上游可能把窗口包在 usage 里，也可能直接放顶层；缺窗口要降级而不是整体失败。
func TestParseOpenCodeGoUsageJSONIsLenient(t *testing.T) {
	t.Parallel()
	wrapped := `{"usage":{"rolling":{"status":"ok","percent":12.5,"resetsAt":"2026-09-24T00:00:00Z"},` +
		`"weekly":{"status":"ok","percent":80},"monthly":{"percent":41.25,"resetsAt":"not-a-time"}}}`
	got, err := parseOpenCodeGoUsageJSON([]byte(wrapped))
	require.NoError(t, err)
	require.NotNil(t, got.Rolling)
	require.InDelta(t, 12.5, got.Rolling.Percent, 0.001)
	require.Equal(t, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), got.Rolling.ResetsAt)
	require.NotNil(t, got.Weekly)
	require.True(t, got.Weekly.ResetsAt.IsZero(), "缺 resetsAt 时为零值，不报错")
	require.NotNil(t, got.Monthly)
	require.True(t, got.Monthly.ResetsAt.IsZero(), "时间格式非法时降级为零值")

	direct := `{"rolling":{"percent":1},"weekly":{"percent":2}}`
	got, err = parseOpenCodeGoUsageJSON([]byte(direct))
	require.NoError(t, err)
	require.NotNil(t, got.Rolling)
	require.Nil(t, got.Monthly, "没有的窗口不要凭空造")

	_, err = parseOpenCodeGoUsageJSON([]byte(`{"usage":`))
	require.Error(t, err, "结构性非法 JSON 才算失败")
}

// 成功路径：写进 extra，且**落库内容里不能出现 API key**。
func TestFetchOpenCodeGoUsagePersistsWindowsWithoutTheKey(t *testing.T) {
	body := `{"usage":{"rolling":{"status":"ok","percent":3,"resetsAt":"2026-09-24T00:00:00Z"},` +
		`"weekly":{"status":"ok","percent":55},"monthly":{"status":"ok","percent":20}}}`
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	acc := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")

	got, err := svc.FetchOpenCodeGoUsage(context.Background(), acc)
	require.NoError(t, err)
	require.NotNil(t, got.Weekly)
	require.InDelta(t, 55, got.Weekly.Percent, 0.001)
	require.Empty(t, got.Error)

	// 请求形状
	require.Equal(t, openCodeGoUsageURL, upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-opencode-go-test-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, http.MethodGet, upstream.lastReq.Method)

	// 落库
	stored := openCodeGoUsageFromExtra(acc)
	require.NotNil(t, stored)
	require.NotNil(t, stored.Monthly)
	raw := acc.GetExtraString(openCodeGoUsageExtraKey)
	require.NotContains(t, raw, "sk-opencode-go-test-key", "落库不得包含 API key")
}

// 失败也要落库（带原因），面板才能区分"没查过"与"查了但失败"。
func TestFetchOpenCodeGoUsageRecordsFailures(t *testing.T) {
	cases := []struct {
		status int
		reason string
	}{
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "Go subscription required (403)"},
		{http.StatusTooManyRequests, "http 429"},
	}
	for _, tc := range cases {
		upstream := &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: tc.status,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			},
		}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		acc := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")

		_, err := svc.FetchOpenCodeGoUsage(context.Background(), acc)
		require.Error(t, err, "status=%d", tc.status)
		stored := openCodeGoUsageFromExtra(acc)
		require.NotNil(t, stored, "失败也必须落库")
		require.Equal(t, tc.reason, stored.Error)
		require.Equal(t, tc.status, stored.HTTPStatus)
		require.NotContains(t, acc.GetExtraString(openCodeGoUsageExtraKey), "sk-opencode-go-test-key")
	}
}

// 非 OpenCode Go 账号必须直接拒绝，绝不发请求（否则可能把 key 发给 opencode.ai）。
func TestFetchOpenCodeGoUsageSkipsUnrelatedAccounts(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	for _, base := range []string{"", "https://relay.example.com/v1", "https://opencode.ai/zen/go/v1x"} {
		acc := openCodeGoTestAccount(base)
		_, err := svc.FetchOpenCodeGoUsage(context.Background(), acc)
		require.ErrorIs(t, err, ErrOpenCodeGoUsageNotApplicable, base)
	}
	require.Nil(t, upstream.lastReq, "不适用时不得发出任何请求")
}
