package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTurnStateHardExpiryBoundary(t *testing.T) {
	now := time.Now()
	for _, raw := range []any{nil, "invalid", now.Add(time.Second), now.Add(-time.Hour)} {
		require.True(t, openAICodexTurnStateTicketExpired(raw, now, time.Hour))
	}
	require.False(t, openAICodexTurnStateTicketExpired(now.Add(-time.Hour+time.Nanosecond), now, time.Hour))
	for _, seconds := range []int{-1, 0, 3600, 86400} {
		cfg := &config.Config{}
		cfg.Gateway.OpenAICodexTurnStateReuseTTLSeconds = seconds
		require.Equal(t, time.Hour, openAICodexTurnStateReuseTTL(cfg))
	}
}

func TestTurnStateFreshMainDoesNotHideStalePools(t *testing.T) {
	now := time.Now()
	tok := "gAAAAA" + strings.Repeat("X", 250)
	acc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	for _, model := range openAICodexTurnStateSeedModels {
		tk, mk, ck := openAICodexTurnStatePoolKeys(model)
		acc.Extra[tk], acc.Extra[mk], acc.Extra[ck] = tok, model, now.Format(time.RFC3339Nano)
	}
	acc.Extra[openAICodexTurnStateExtraToken] = tok
	acc.Extra[openAICodexTurnStateExtraModel] = "gpt-6-astra"
	acc.Extra[openAICodexTurnStateExtraCapturedAt] = now.Format(time.RFC3339Nano)
	require.Empty(t, openAICodexTurnStateModelsNeedingRefresh(acc, now, time.Hour))
	_, _, ck := openAICodexTurnStatePoolKeys("gpt-5.6-terra")
	acc.Extra[ck] = now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
	require.Equal(t, []string{"gpt-5.6-terra"}, openAICodexTurnStateModelsNeedingRefresh(acc, now, time.Hour))
	require.Empty(t, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-5.6-terra", now, time.Hour), "must not borrow the fresh astra ticket")
}

func TestTurnStateSameTicketDoesNotExtendLifetime(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{}}
	tok := "gAAAAA" + strings.Repeat("Y", 250)
	svc.storeOpenAICodexTurnStateForPool(acc, "gpt-5.6-sol", tok, "gpt-5.6-sol")
	_, _, ck := openAICodexTurnStatePoolKeys("gpt-5.6-sol")
	old := time.Now().Add(-50 * time.Minute).Format(time.RFC3339Nano)
	acc.Extra[ck], acc.Extra[openAICodexTurnStateExtraCapturedAt] = old, old
	svc.storeOpenAICodexTurnStateForPool(acc, "gpt-5.6-sol", tok, "gpt-5.6-sol")
	svc.captureOpenAICodexTurnState(acc, http.Header{}, "gpt-5.6-sol", true)
	require.Equal(t, old, acc.Extra[ck])
	require.Equal(t, old, acc.Extra[openAICodexTurnStateExtraCapturedAt])
}

func TestTurnStateRefreshDeduplicatedAndCooledDown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	now := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.claimOpenAICodexTurnStateRefresh(69, now) {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1, accepted)
	require.True(t, svc.claimOpenAICodexTurnStateRefresh(112, now))
	svc.finishOpenAICodexTurnStateRefresh(69, now)
	require.False(t, svc.claimOpenAICodexTurnStateRefresh(69, now.Add(time.Minute)))
	require.True(t, svc.claimOpenAICodexTurnStateRefresh(69, now.Add(2*time.Minute)))
}

type turnStateRoundTripper func(*http.Request) (*http.Response, error)

func (f turnStateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTurnStateProbeRequiresCompletionAndUsesResponseModel(t *testing.T) {
	tok := "gAAAAA" + strings.Repeat("Z", 250)
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"complete", `data: {"type":"response.completed","response":{"model":"gpt-5.6-luna"}}`, 200, true},
		{"failed", `data: {"type":"response.failed"}`, 200, false},
		{"http_error", `data: {"type":"response.completed"}`, 502, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: turnStateRoundTripper(func(r *http.Request) (*http.Response, error) {
				require.Empty(t, r.Header.Get(openAICodexTurnStateHeader))
				require.Equal(t, codexCLIVersion, r.Header.Get("version"))
				h := http.Header{}
				h.Set(openAICodexTurnStateHeader, tok)
				return &http.Response{StatusCode: tc.status, Header: h, Body: io.NopCloser(strings.NewReader(tc.body + "\n\n"))}, nil
			})}
			result := probeOpenAICodexTurnState(context.Background(), client, "test-token", "test-id", "gpt-6-astra")
			require.Equal(t, tc.ok, result.completed)
			if tc.ok {
				require.Equal(t, "gpt-5.6-luna", result.model)
			}
		})
	}
}

func TestTurnStateSeedRefreshesEveryPoolAndPersistsAtomically(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	repo := &turnStatePersistRepo{}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: repo}
	acc := &Account{ID: 69, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}, Credentials: map[string]any{"access_token": "test", "chatgpt_account_id": "test"}}
	var models []string
	client := &http.Client{Transport: turnStateRoundTripper(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		model := gjson.GetBytes(body, "model").String()
		models = append(models, model)
		h := http.Header{}
		h.Set(openAICodexTurnStateHeader, "gAAAAA"+strings.Repeat("T", 250))
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"model":"` + model + `"}}` + "\n\n"))}, nil
	})}
	svc.seedOpenAICodexTurnStateWithClient(context.Background(), acc, client)
	require.Equal(t, openAICodexTurnStateSeedModels[:len(openAICodexTurnStateSeedModels)-1], models, "review alias reuses the luna pool seeded earlier in this job")
	require.Len(t, repo.updates, len(models))
	for i, model := range models {
		tk, _, ck := openAICodexTurnStatePoolKeys(model)
		require.NotEmpty(t, repo.updates[i][tk])
		require.Equal(t, repo.updates[i][ck], repo.updates[i][openAICodexTurnStateExtraCapturedAt])
	}
	svc.seedOpenAICodexTurnStateWithClient(context.Background(), acc, client)
	require.Len(t, models, len(openAICodexTurnStateSeedModels)-1, "fresh DB state must suppress queued reprobes")
}

type turnStatePersistRepo struct {
	ports.AccountRepository
	updates []map[string]any
}

func (r *turnStatePersistRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = append(r.updates, updates)
	return nil
}

func TestTurnStateConsecutiveFailuresAndRecovery(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	acc := &Account{ID: 69, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	tok := "gAAAAA" + strings.Repeat("T", 250)
	svc.storeOpenAICodexTurnState(acc, tok, "gpt-5.6-sol")
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	require.NotEmpty(t, acc.Extra[openAICodexTurnStateExtraToken])
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 200)
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	require.NotEmpty(t, acc.Extra[openAICodexTurnStateExtraToken])
	svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraToken])
	svc.storeOpenAICodexTurnState(acc, tok, "gpt-5.6-sol")
	for i := 0; i < 3; i++ {
		svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502)
	}
	require.NotEmpty(t, acc.Extra[openAICodexTurnStateExtraToken], "cooldown must prevent repeated invalidation of freshly seeded tickets")
}

func TestTurnStateTransportErrorDoesNotInvalidate(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg}
	acc := &Account{ID: 69, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	tok := "gAAAAA" + strings.Repeat("T", 250)
	svc.storeOpenAICodexTurnState(acc, tok, "gpt-5.6-sol")
	// transport 错误（proxy/DNS/TLS 超时）被 handler 包装成 failover.StatusCode=502，
	// 但上游未返回真实 HTTP 响应，不得计入连续 5xx，更不得清票。
	for i := 0; i < 5; i++ {
		svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502, true /* transportOnly */)
	}
	require.NotEmpty(t, acc.Extra[openAICodexTurnStateExtraToken],
		"transport-only errors must not invalidate cached turn-state tickets")
	// 真实上游 5xx（transportOnly=false）仍按阈值清票。
	for i := 0; i < 3; i++ {
		svc.NoteOpenAICodexTurnStateUpstreamResult(acc, 502, false)
	}
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraToken])
}

func TestTurnStateConcurrent5xxInvalidatesOncePerCooldown(t *testing.T) {
	svc := &OpenAIGatewayService{}
	now := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	invalidations := 0
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.shouldInvalidateOpenAICodexTurnState(69, 502, now) {
				mu.Lock()
				invalidations++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1, invalidations)
	require.False(t, svc.shouldInvalidateOpenAICodexTurnState(69, 502, now.Add(2*time.Minute)))
	require.False(t, svc.shouldInvalidateOpenAICodexTurnState(69, 502, now.Add(2*time.Minute)))
	require.True(t, svc.shouldInvalidateOpenAICodexTurnState(69, 502, now.Add(2*time.Minute)))
}
