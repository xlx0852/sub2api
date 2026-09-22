package service

import (
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsReusableOpenAICodexTurnState(t *testing.T) {
	t.Parallel()
	ok292 := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("A", 292-32)
	require.True(t, isReusableOpenAICodexTurnState(ok292))
	require.False(t, isReusableOpenAICodexTurnState(""))
	require.False(t, isReusableOpenAICodexTurnState("short"))
	require.False(t, isReusableOpenAICodexTurnState("notfernet"+stringsRepeat("B", 280)))
	require.False(t, isReusableOpenAICodexTurnState(ok292[:100]+" "+ok292[100:]))
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, s...)
	}
	return string(out[:n])
}

func TestLookupCachedOpenAICodexTurnStateTTL(t *testing.T) {
	t.Parallel()
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("C", 260)
	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
	}}
	require.Equal(t, token, lookupCachedOpenAICodexTurnState(acc, time.Now(), 30*time.Minute))
	// 票龄 10min > ttl=5min → 硬失效，不再返回旧票（SWR 已改为过期即废）。
	require.Empty(t, lookupCachedOpenAICodexTurnState(acc, time.Now(), 5*time.Minute))
	require.True(t, openAICodexTurnStateNeedsRefresh(acc, time.Now(), 5*time.Minute))
	acc.Type = AccountTypeAPIKey
	require.Empty(t, lookupCachedOpenAICodexTurnState(acc, time.Now(), 30*time.Minute))
}

func TestApplyOpenAICodexTurnStateReuseOverridesClientHeader(t *testing.T) {
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("D", 260)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}
	h := http.Header{}
	svc.applyOpenAICodexTurnStateReuse(h, acc)
	require.Equal(t, token, h.Get(openAICodexTurnStateHeader))

	h2 := http.Header{}
	h2.Set(openAICodexTurnStateHeader, "client-supplied")
	svc.applyOpenAICodexTurnStateReuse(h2, acc)
	require.Equal(t, token, h2.Get(openAICodexTurnStateHeader))

	h3 := http.Header{}
	h3.Set(openAICodexTurnStateHeader, "client-supplied")
	svc.applyOpenAICodexTurnStateReuse(h3, &Account{Type: AccountTypeOAuth})
	require.Empty(t, h3.Get(openAICodexTurnStateHeader))
}

func TestCaptureOpenAICodexTurnStateOnlyOnSuccess(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	acc := &Account{ID: 7, Type: AccountTypeOAuth, Extra: map[string]any{}}
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("E", 260)
	h := http.Header{}
	h.Set(openAICodexTurnStateHeader, token)

	svc.captureOpenAICodexTurnState(acc, h, "gpt-6-astra", false)
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraToken])

	svc.captureOpenAICodexTurnState(acc, h, "gpt-6-astra", true)
	require.Equal(t, token, acc.Extra[openAICodexTurnStateExtraToken])
	require.Equal(t, "gpt-6-astra", acc.Extra[openAICodexTurnStateExtraModel])
}

func TestSortOpenAICodexTurnStateSeedQueueFIFO(t *testing.T) {
	t.Parallel()
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)
	accounts := []Account{
		{ID: 3, CreatedAt: newer},
		{ID: 1, CreatedAt: older},
		{ID: 2, CreatedAt: older},
	}
	sortOpenAICodexTurnStateSeedQueue(accounts)
	require.Equal(t, int64(1), accounts[0].ID)
	require.Equal(t, int64(2), accounts[1].ID)
	require.Equal(t, int64(3), accounts[2].ID)
}

func TestOpenAICodexTurnStateNeedsRefreshBeforeTTL(t *testing.T) {
	t.Parallel()
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("R", 260)
	fresh := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano),
	}}
	stale := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Add(-26 * time.Minute).Format(time.RFC3339Nano),
	}}
	ttl := 30 * time.Minute
	require.False(t, openAICodexTurnStateNeedsRefresh(fresh, time.Now(), ttl))
	require.True(t, openAICodexTurnStateNeedsRefresh(stale, time.Now(), ttl))
}

func TestCapacityDoesNotWipeCachedTurnState(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("H", 260)
	acc := &Account{ID: 11, Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}
	svc.scheduleOpenAICodexTurnStateRefresh(acc)
	require.Equal(t, token, acc.Extra[openAICodexTurnStateExtraToken])
}

func TestInvalidateOpenAICodexTurnStateClearsCache(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("G", 260)
	acc := &Account{ID: 9, Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}
	svc.invalidateOpenAICodexTurnState(acc)
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraToken])
	require.Empty(t, lookupCachedOpenAICodexTurnState(acc, time.Now(), 30*time.Minute))
}

func TestOpenAICodexTurnStateCandidatePools(t *testing.T) {
	t.Parallel()
	// astra 请求查两个槽：本名槽优先，其次 luna 别名槽
	require.Equal(t, []string{"gpt-6-astra", "gpt-5.6-luna"}, openAICodexTurnStateCandidatePools("gpt-6-astra"))
	// 其他 model 只查自身槽
	require.Equal(t, []string{"gpt-5.6-luna"}, openAICodexTurnStateCandidatePools("gpt-5.6-luna"))
	require.Equal(t, []string{"gpt-5.6-sol"}, openAICodexTurnStateCandidatePools("gpt-5.6-sol"))
	require.Equal(t, []string{"gpt-5.6-terra"}, openAICodexTurnStateCandidatePools("gpt-5.6-terra"))
	require.Nil(t, openAICodexTurnStateCandidatePools(""))
}

func TestPoolTicketIsolation(t *testing.T) {
	t.Parallel()
	solTok := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("S", 260)
	lunaTok := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("L", 260)
	now := time.Now()
	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol":              solTok,
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_model":        "gpt-5.6-sol",
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_captured_at":  now.Format(time.RFC3339Nano),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna":             lunaTok,
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna_model":       "gpt-5.6-luna",
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna_captured_at": now.Format(time.RFC3339Nano),
	}}
	// 按池查
	require.Equal(t, lunaTok, lookupCachedOpenAICodexTurnStateForPool(acc, "gpt-5.6-luna", now, 30*time.Minute))
	require.Equal(t, solTok, lookupCachedOpenAICodexTurnStateForPool(acc, "gpt-5.6-sol", now, 30*time.Minute))
	// terra 槽空 → 回退单值（此处单值不存在 → 空）
	require.Empty(t, lookupCachedOpenAICodexTurnStateForPool(acc, "gpt-5.6-terra", now, 30*time.Minute))
	// astra 请求：本名槽空 → 落 luna 槽
	require.Equal(t, lunaTok, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-6-astra", now, 30*time.Minute))
}

func TestPoolTicketFallsBackToLegacySingle(t *testing.T) {
	t.Parallel()
	legacy := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("T", 260)
	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      legacy,
		openAICodexTurnStateExtraCapturedAt: time.Now().Format(time.RFC3339Nano),
	}}
	// Unknown/cross-pool legacy tickets must not bypass pool isolation.
	require.Empty(t, lookupCachedOpenAICodexTurnStateForPool(acc, "gpt-5.6-sol", time.Now(), time.Hour))
	require.Empty(t, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-5.6-sol", time.Now(), time.Hour))
	acc.Extra[openAICodexTurnStateExtraModel] = "gpt-5.6-sol"
	require.Equal(t, legacy, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-5.6-sol", time.Now(), time.Hour))
	require.Empty(t, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-5.6-terra", time.Now(), time.Hour))
}

func TestCaptureStoresToPoolAndLegacy(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	acc := &Account{ID: 21, Type: AccountTypeOAuth, Extra: map[string]any{}}
	tok := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("M", 260)
	h := http.Header{}
	h.Set(openAICodexTurnStateHeader, tok)
	// resp_model=gpt-5.6-luna → 归 luna 槽
	svc.captureOpenAICodexTurnState(acc, h, "gpt-5.6-luna", true)
	tokKey, _, _ := openAICodexTurnStatePoolKeys("gpt-5.6-luna")
	require.Equal(t, tok, acc.Extra[tokKey])
	require.Equal(t, tok, acc.Extra[openAICodexTurnStateExtraToken])

	// resp_model=gpt-6-astra → 归 astra 本名槽
	acc2 := &Account{ID: 22, Type: AccountTypeOAuth, Extra: map[string]any{}}
	tok2 := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("N", 260)
	h2 := http.Header{}
	h2.Set(openAICodexTurnStateHeader, tok2)
	svc.captureOpenAICodexTurnState(acc2, h2, "gpt-6-astra", true)
	astraKey, _, _ := openAICodexTurnStatePoolKeys("gpt-6-astra")
	require.Equal(t, tok2, acc2.Extra[astraKey])
}

func TestIsOpenAICodexTurnStateRevokedResponse(t *testing.T) {
	t.Parallel()
	// HTTP 312 状态码命中
	require.True(t, isOpenAICodexTurnStateRevokedResponse(312, nil))
	// 流内 error.code=312
	require.True(t, isOpenAICodexTurnStateRevokedResponse(200, []byte(`{"type":"response.failed","response":{"error":{"code":"312"}}}`)))
	// degraded marker
	require.True(t, isOpenAICodexTurnStateRevokedResponse(200, []byte(`{"type":"error","error":{"code":"turn_state_degraded"}}`)))
	// state_revoked
	require.True(t, isOpenAICodexTurnStateRevokedResponse(200, []byte(`{"type":"response.failed","response":{"error":{"code":"state_revoked"}}`)))
	// 普通 overloaded 不是 312
	require.False(t, isOpenAICodexTurnStateRevokedResponse(200, []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}`)))
	// 空 payload / 200 无 marker 不命中
	require.False(t, isOpenAICodexTurnStateRevokedResponse(200, nil))
	require.False(t, isOpenAICodexTurnStateRevokedResponse(200, []byte(`{"type":"response.completed"}`)))
	// 非 312 状态码不命中
	require.False(t, isOpenAICodexTurnStateRevokedResponse(200, nil))
	require.False(t, isOpenAICodexTurnStateRevokedResponse(500, nil))
}

func TestInvalidateClearsAllPoolSlots(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	tok := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("X", 260)
	acc := &Account{ID: 31, Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:                                  tok,
		openAICodexTurnStateExtraCapturedAt:                             time.Now().Format(time.RFC3339Nano),
		openAICodexTurnStateExtraPoolPrefix + "gpt-6-astra":             tok,
		openAICodexTurnStateExtraPoolPrefix + "gpt-6-astra_model":       "gpt-6-astra",
		openAICodexTurnStateExtraPoolPrefix + "gpt-6-astra_captured_at": time.Now().Format(time.RFC3339Nano),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol":             tok,
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_model":       "gpt-5.6-sol",
	}}
	svc.invalidateOpenAICodexTurnState(acc)
	// 单值与所有池槽位全清
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraToken])
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraPoolPrefix+"gpt-6-astra"])
	require.Empty(t, acc.Extra[openAICodexTurnStateExtraPoolPrefix+"gpt-5.6-sol"])
	require.Empty(t, lookupCachedOpenAICodexTurnStateForModel(acc, "gpt-6-astra", time.Now(), 30*time.Minute))
}

func TestCaptureOpenAICodexTurnStateDoesNotRenewWithoutNewHeader(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	token := "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat("F", 260)
	old := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	acc := &Account{ID: 8, Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      token,
		openAICodexTurnStateExtraCapturedAt: old,
	}}
	svc.captureOpenAICodexTurnState(acc, http.Header{}, "gpt-6-astra", true)
	got, _ := acc.Extra[openAICodexTurnStateExtraCapturedAt].(string)
	require.Equal(t, old, got)
	require.Equal(t, token, acc.Extra[openAICodexTurnStateExtraToken])
}

// parseTurnStateAgeSummary 从诊断串取出年龄区间（秒）。
func parseTurnStateAgeSummary(t *testing.T, summary string) (float64, float64) {
	t.Helper()
	m := regexp.MustCompile(`age_min_s=([0-9.]+) age_max_s=([0-9.]+)`).FindStringSubmatch(summary)
	require.Len(t, m, 3, "summary=%q", summary)
	minAge, err := strconv.ParseFloat(m[1], 64)
	require.NoError(t, err)
	maxAge, err := strconv.ParseFloat(m[2], 64)
	require.NoError(t, err)
	return minAge, maxAge
}

// 票龄诊断必须回答"出事时被回放的票有多旧"，因此要覆盖：池票优先、
// 单值兜底不重复计数、以及无法建立票龄的槽位不计入。
func TestOpenAICodexTurnStateAgeSummary(t *testing.T) {
	t.Parallel()
	tok := func(c string) string {
		return "gAAAAABqq_20UDF7Ymoy_iFdYwpAR8Uz" + stringsRepeat(c, 260)
	}
	now := time.Now()

	require.Equal(t, "tickets=none", openAICodexTurnStateAgeSummary(nil, now))
	require.Equal(t, "tickets=none", openAICodexTurnStateAgeSummary(&Account{Type: AccountTypeOAuth}, now))

	acc := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna":             tok("L"),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna_model":       "gpt-5.6-luna",
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna_captured_at": now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol":              tok("S"),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_model":        "gpt-5.6-sol",
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_captured_at":  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		// 单值槽位持有最新票，但已被池槽位覆盖，不得重复计数。
		openAICodexTurnStateExtraToken:      tok("L"),
		openAICodexTurnStateExtraCapturedAt: now.Format(time.RFC3339Nano),
	}}
	summary := openAICodexTurnStateAgeSummary(acc, now)
	require.Contains(t, summary, "tickets=2")
	minAge, maxAge := parseTurnStateAgeSummary(t, summary)
	require.InDelta(t, 600, minAge, 2)
	require.InDelta(t, 1800, maxAge, 2)

	// 没有任何池票时才回退兼容单值槽位。
	legacy := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraToken:      tok("T"),
		openAICodexTurnStateExtraCapturedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
	}}
	summary = openAICodexTurnStateAgeSummary(legacy, now)
	require.Contains(t, summary, "tickets=1")
	minAge, maxAge = parseTurnStateAgeSummary(t, summary)
	require.InDelta(t, 300, minAge, 2)
	require.InDelta(t, 300, maxAge, 2)

	// 空票、未来时间（时钟回拨）、缺失时间都建立不了票龄。
	bad := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna":             "",
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-luna_captured_at": now.Format(time.RFC3339Nano),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol":              tok("S"),
		openAICodexTurnStateExtraPoolPrefix + "gpt-5.6-sol_captured_at":  now.Add(time.Hour).Format(time.RFC3339Nano),
	}}
	require.Equal(t, "tickets=none", openAICodexTurnStateAgeSummary(bad, now))
}
