package service

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newCodexCookieTestService() *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	return &OpenAIGatewayService{cfg: cfg}
}

func newCodexCookieTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Extra:       map[string]any{},
		Credentials: map[string]any{"access_token": "tok", "chatgpt_account_id": "cid"},
	}
}

func codexCookieTestHeaders(setCookies ...string) http.Header {
	h := http.Header{}
	h.Set(openAICodexTurnStateHeader, "gAAAAA"+strings.Repeat("C", 250))
	for _, c := range setCookies {
		h.Add("Set-Cookie", c)
	}
	return h
}

// 采集按上游给的寿命走：__oailb 那种 Max-Age 与 __cf_bm 那种 Expires 都要认；
// 且必须能覆盖同名 cookie（上游只下发变化的那几个）。
func TestCodexTurnStateCookieCaptureHonorsUpstreamLifetime(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9001)
	now := time.Now().UTC()

	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=lb-one; Max-Age=3600; Path=/; Secure; HttpOnly",
		"__cf_bm=cf-one; Expires="+now.Add(20*time.Minute).UTC().Format(http.TimeFormat)+"; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)

	got := codexTurnStateCookieHeaderValue(acc, now)
	require.Contains(t, got, "__oailb=lb-one")
	require.Contains(t, got, "__cf_bm=cf-one")
	require.Equal(t, 2, codexTurnStateCookieCount(acc, now))

	// 一小时零一秒后：__oailb 按 Max-Age 到期，__cf_bm 早在 20 分钟时就到期了。
	require.Empty(t, codexTurnStateCookieHeaderValue(acc, now.Add(61*time.Minute)))

	// 同名覆盖：只换 __oailb，__cf_bm 保持原值。
	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=lb-two; Max-Age=3600; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)
	got = codexTurnStateCookieHeaderValue(acc, now)
	require.Contains(t, got, "__oailb=lb-two")
	require.Contains(t, got, "__cf_bm=cf-one")
}

// 被拒/失败的响应不得刷新 cookie——否则一次失败就把上一份可用会话冲掉。
func TestCodexTurnStateCookiesOnlyRefreshFromAcceptedResponses(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9002)

	accepted := codexCookieTestHeaders("__oailb=good; Max-Age=3600; Path=/")
	svc.captureOpenAICodexTurnStateForPool(acc, accepted, "gpt-5.6-luna", "gpt-5.6-luna", true)
	require.Contains(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "__oailb=good")

	// 有 Set-Cookie 但**没有可复用票**（被拒/失败）→ 不得改写已存的 cookie。
	rejected := http.Header{}
	rejected.Add("Set-Cookie", "__oailb=bad; Max-Age=3600; Path=/")
	svc.captureOpenAICodexTurnStateForPool(acc, rejected, "gpt-5.6-luna", "gpt-5.6-luna", true)
	require.Contains(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "__oailb=good")

	// success=false 同理。
	failed := codexCookieTestHeaders("__oailb=worse; Max-Age=3600; Path=/")
	svc.captureOpenAICodexTurnStateForPool(acc, failed, "gpt-5.6-luna", "gpt-5.6-luna", false)
	require.Contains(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "__oailb=good")
}

// cookie 是绑定出口的亲和令牌：出口变了就不回放，否则是拿旧出口的亲和去新出口用。
func TestCodexTurnStateCookieIsBoundToEgress(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9003)

	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=direct-lb; Max-Age=3600; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)
	require.NotEmpty(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "直连采集的 cookie 应可用")

	// 换成账号代理 → 出口指纹变化 → 不再回放。
	acc.Proxy = &Proxy{Protocol: "http", Host: "proxy.internal", Port: 8080}
	require.Empty(t, codexTurnStateCookieHeaderValue(acc, time.Now()))
	require.Equal(t, 0, codexTurnStateCookieCount(acc, time.Now()))
}

// Max-Age<=0 是上游在删除该 cookie，不该继续回放。
func TestCodexTurnStateCookieDeletionIsHonored(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9004)
	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=lb; Max-Age=3600; Path=/",
		"__cf_bm=cf; Max-Age=600; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)
	require.Contains(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "__cf_bm=cf")

	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__cf_bm=; Max-Age=0; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)
	got := codexTurnStateCookieHeaderValue(acc, time.Now())
	require.Contains(t, got, "__oailb=lb")
	require.NotContains(t, got, "__cf_bm")
}

// 失效语义是「这个会话不要了」：票与 cookie 必须一起清，
// 否则过载后只换票不换后端，等于继续钉在已饱和的后端上。
func TestInvalidateClearsTurnStateCookies(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9005)
	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=lb; Max-Age=3600; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)
	require.NotEmpty(t, codexTurnStateCookieHeaderValue(acc, time.Now()))

	svc.invalidateOpenAICodexTurnState(acc)

	require.Empty(t, codexTurnStateCookieHeaderValue(acc, time.Now()))
	require.Nil(t, acc.Extra[openAICodexTurnStateCookieExtra])
	require.Nil(t, acc.Extra[openAICodexTurnStateCookieEgressExtra])
}

// 诊断串里绝不能出现 cookie 值。
func TestCodexTurnStateCookieDiagnosticsNeverExposeValues(t *testing.T) {
	svc := newCodexCookieTestService()
	acc := newCodexCookieTestAccount(9006)
	svc.captureOpenAICodexTurnStateForPool(acc, codexCookieTestHeaders(
		"__oailb=SUPER-SECRET-LB-VALUE; Max-Age=3600; Path=/",
	), "gpt-5.6-luna", "gpt-5.6-luna", true)

	require.Equal(t, 1, codexTurnStateCookieCount(acc, time.Now()))
	// 年龄诊断串同样不得夹带 cookie 值。
	require.NotContains(t, openAICodexTurnStateAgeSummary(acc, time.Now()), "SUPER-SECRET-LB-VALUE")
}
