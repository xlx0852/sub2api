package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 这次踩的坑正是"函数写对了但没接到真正被调用的路径上"（我错挂到了另一个探测函数）。
// 因此这里**走真实的账号测试入口**断言出站头，而不是只测 apply 函数本身。
//
// 自包含：不依赖带 `unit` 构建标签的那套 helper——那套当前在共享分支上因其它测试
// 的文件签名不匹配而无法编译（与本次改动无关），而这条断言恰恰最需要能跑起来。
type openCodeTestUpstream struct {
	requests []*http.Request
	resp     *http.Response
}

func (u *openCodeTestUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.resp, nil
}

func (u *openCodeTestUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	return u.resp, nil
}

// openCodeTestRepo 只实现本用例会走到的方法。
type openCodeTestRepo struct {
	ports.AccountRepository
	updates []map[string]any
}

func (r *openCodeTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = append(r.updates, updates)
	return nil
}

func newOpenCodeAccountTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/138/test", nil)
	return c
}

func TestAccountTestService_OpenCodeGoSendsSessionHeader(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"type":"error"}`)),
	}
	upstream := &openCodeTestUpstream{resp: resp}
	svc := &AccountTestService{cfg: &config.Config{}, accountRepo: &openCodeTestRepo{}, httpUpstream: upstream}
	account := &Account{
		ID:          138,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "oc_sk_test_key",
			"base_url": "https://opencode.ai/zen/go/v1",
		},
	}

	ctx := newOpenCodeAccountTestContext()
	err := svc.testOpenAIAccountConnection(ctx, account, "deepseek-flash", "hi", "")
	t.Logf("err=%v", err)

	require.NotEmpty(t, upstream.requests, "应发出测试请求")
	req := upstream.requests[0]
	require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", req.URL.String(),
		"DeepSeek Go 模型必须走 Chat Completions，而不是 Responses")
	require.NotEmpty(t, req.Header.Get(openCodeSessionHeader),
		"官方 OpenCode 主机缺 x-opencode-session 会被上游 400 MissingSessionID")
	require.Equal(t, openCodeUpstreamUserAgent, req.Header.Get("User-Agent"),
		"发往 opencode.ai 的 UA 必须是 opencode 客户端形态，否则 Cloudflare 1010 会拦成 403")
	require.Equal(t, "Bearer oc_sk_test_key", req.Header.Get("Authorization"))
}

// 非 OpenCode 账号不得被塞入该头，也不得改写 UA（避免污染其它上游的指纹）。
func TestAccountTestService_NonOpenCodeAccountKeepsHeadersUntouched(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"type":"error"}`)),
	}
	upstream := &openCodeTestUpstream{resp: resp}
	svc := &AccountTestService{cfg: &config.Config{}, accountRepo: &openCodeTestRepo{}, httpUpstream: upstream}
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-normal"},
	}

	_ = svc.testOpenAIAccountConnection(newOpenCodeAccountTestContext(), account, "gpt-5.4", "hi", "")

	require.NotEmpty(t, upstream.requests)
	require.Empty(t, upstream.requests[0].Header.Get(openCodeSessionHeader))
	require.NotEqual(t, openCodeUpstreamUserAgent, upstream.requests[0].Header.Get("User-Agent"))
}
