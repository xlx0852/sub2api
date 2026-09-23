package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/stretchr/testify/require"
)

func openCodeGoAccountWithSnapshot(t *testing.T, fetchedAt time.Time, errText string) *Account {
	t.Helper()
	acc := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")
	snapshot := OpenCodeGoUsage{FetchedAt: fetchedAt, Error: errText}
	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	acc.Extra[openCodeGoUsageExtraKey] = string(raw)
	return acc
}

func TestOpenCodeGoUsageNeedsRefresh(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	stale := openCodeGoUsageStaleAfter

	// 没有快照 → 需要
	require.True(t, openCodeGoUsageNeedsRefresh(openCodeGoTestAccount("https://opencode.ai/zen/go/v1"), now, stale))
	// 刚查过 → 不需要
	require.False(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(-2*time.Minute), ""), now, stale))
	// 超过阈值 → 需要
	require.True(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(-16*time.Minute), ""), now, stale))
	// 失败也参与计时：失败不该变成"每次都重试"
	require.False(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(-3*time.Minute), "unauthorized"), now, stale))
	require.True(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(-20*time.Minute), "unauthorized"), now, stale))
	// 时间在未来（时钟回拨/脏数据）→ 重查，别把账号永久钉死
	require.True(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(time.Hour), ""), now, stale))
	// 阈值非法时退回默认值，不 panic
	require.True(t, openCodeGoUsageNeedsRefresh(openCodeGoAccountWithSnapshot(t, now.Add(-16*time.Minute), ""), now, 0))
}

// 扫描必须只碰 OpenCode Go 账号：判定错了就会把别家的 key 发给 opencode.ai。
func TestRefreshOpenCodeGoUsageOnlyTouchesMatchingAccounts(t *testing.T) {
	opencode := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")
	opencode.ID = 1
	relay := openCodeGoTestAccount("https://relay.example.com/v1")
	relay.ID = 2
	relay.Credentials["api_key"] = "sk-relay-should-never-be-sent"
	oauth := openCodeGoTestAccount("https://opencode.ai/zen/go/v1")
	oauth.ID = 3
	oauth.Type = AccountTypeOAuth

	repo := &openCodeGoListRepo{accounts: []Account{*opencode, *relay, *oauth}}
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"weekly":{"percent":10}}}`)),
		},
	}
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream}

	svc.refreshOpenCodeGoUsageAccounts(context.Background())

	require.Len(t, upstream.requests, 1, "只应对 OpenCode Go 账号发起一次请求")
	require.Equal(t, "Bearer sk-opencode-go-test-key", upstream.requests[0].Header.Get("Authorization"))
	require.NotContains(t, upstream.requests[0].Header.Get("Authorization"), "relay-should-never-be-sent")
}

// 嵌入端口接口：只实现本用例真正会走到的方法，其余保持未实现（被调用即 panic，暴露误用）。
type openCodeGoListRepo struct {
	ports.AccountRepository
	accounts []Account
	updates  []map[string]any
}

func (r *openCodeGoListRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return r.accounts, nil
}

func (r *openCodeGoListRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = append(r.updates, updates)
	return nil
}
