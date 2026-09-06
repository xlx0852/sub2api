package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// antigravityFailingWriter 模拟客户端断开连接的 gin.ResponseWriter
type antigravityFailingWriter struct {
	gin.ResponseWriter
	failAfter int // 允许成功写入的次数，之后所有写入返回错误
	writes    int
}

func (w *antigravityFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

// newAntigravityTestService 创建用于流式测试的 AntigravityGatewayService
func newAntigravityTestService(cfg *config.Config) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		settingService: &SettingService{cfg: cfg},
	}
}

func TestAntigravityUpstreamErrorBodyReadLimit_RespectsDiagnosticLimit(t *testing.T) {
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{
		LogUpstreamErrorBody:         true,
		LogUpstreamErrorBodyMaxBytes: int(gatewayUpstreamErrorBodyReadLimit) + 1024,
	}})

	require.Equal(t, int64(svc.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes), svc.upstreamErrorBodyReadLimit())
}

func TestIsPromptTooLongError(t *testing.T) {
	require.True(t, isPromptTooLongError([]byte(`{"error":{"message":"Prompt is too long"}}`)))
	require.True(t, isPromptTooLongError([]byte(`{"message":"Prompt is too long"}`)))
	require.False(t, isPromptTooLongError([]byte(`{"error":{"message":"other"}}`)))
}

type httpUpstreamStub struct {
	resp *http.Response
	err  error
}

func (s *httpUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.resp, s.err
}

func (s *httpUpstreamStub) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.resp, s.err
}

type queuedHTTPUpstreamStub struct {
	responses     []*http.Response
	errors        []error
	requestBodies [][]byte
	callCount     int
	onCall        func(*http.Request, *queuedHTTPUpstreamStub)
}

func (s *queuedHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		s.requestBodies = append(s.requestBodies, body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	} else {
		s.requestBodies = append(s.requestBodies, nil)
	}

	idx := s.callCount
	s.callCount++
	if s.onCall != nil {
		s.onCall(req, s)
	}

	var resp *http.Response
	if idx < len(s.responses) {
		resp = s.responses[idx]
	}
	var err error
	if idx < len(s.errors) {
		err = s.errors[idx]
	}
	if resp == nil && err == nil {
		return nil, errors.New("unexpected upstream call")
	}
	return resp, err
}

func (s *queuedHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

type recordingInternal500CounterCache struct {
	incrementCalls []int64
	resetCalls     []int64
}

func (c *recordingInternal500CounterCache) IncrementInternal500Count(_ context.Context, accountID int64) (int64, error) {
	c.incrementCalls = append(c.incrementCalls, accountID)
	return int64(len(c.incrementCalls)), nil
}

func (c *recordingInternal500CounterCache) ResetInternal500Count(_ context.Context, accountID int64) error {
	c.resetCalls = append(c.resetCalls, accountID)
	return nil
}

type antigravitySettingRepoStub struct{}

func (s *antigravitySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *antigravitySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	return "", ErrSettingNotFound
}

func (s *antigravitySettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *antigravitySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *antigravitySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *antigravitySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *antigravitySettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestResolveAntigravityProjectID(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
		wantErr bool
	}{
		{
			name: "uses onboard project_id first",
			account: &Account{Credentials: map[string]any{
				"project_id": " onboard-project ",
				antigravityProjectIDFallbackCredentialKey: " configured-project ",
			}},
			want: "onboard-project",
		},
		{
			name: "uses configured credentials fallback",
			account: &Account{Credentials: map[string]any{
				antigravityProjectIDFallbackCredentialKey: " configured-project ",
			}},
			want: "configured-project",
		},
		{
			name: "uses configured extra fallback",
			account: &Account{Extra: map[string]any{
				antigravityProjectIDFallbackCredentialKey: " extra-project ",
			}},
			want: "extra-project",
		},
		{
			name:    "missing project",
			account: &Account{Credentials: map[string]any{}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAntigravityProjectID(tc.account)
			if tc.wantErr {
				require.ErrorIs(t, err, errAntigravityProjectIDRequired)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAntigravityGatewayService_ForwardGemini_UsesConfiguredProjectFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
		},
	})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(body))

	upstreamBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}}\n\n")
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
			},
		},
	}
	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}

	account := &Account{
		ID:          101,
		Name:        "acc-configured-project",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			antigravityProjectIDFallbackCredentialKey: "configured-project",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)

	var wrapped map[string]any
	require.NoError(t, json.Unmarshal(upstream.requestBodies[0], &wrapped))
	require.Equal(t, "configured-project", wrapped["project"])
}

func TestAntigravityGatewayService_ForwardGemini_MissingProjectReturnsLocalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
		},
	})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(body))

	upstream := &queuedHTTPUpstreamStub{}
	internal500Cache := &recordingInternal500CounterCache{}
	svc := &AntigravityGatewayService{
		tokenProvider:    &AntigravityTokenProvider{},
		httpUpstream:     upstream,
		internal500Cache: internal500Cache,
	}

	account := &Account{
		ID:          102,
		Name:        "acc-missing-project",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, body, false)
	require.Nil(t, result)
	require.ErrorIs(t, err, errAntigravityProjectIDRequired)
	require.Equal(t, http.StatusBadRequest, writer.Code)
	require.Empty(t, upstream.requestBodies)
	require.Empty(t, internal500Cache.incrementCalls)
	require.Contains(t, writer.Body.String(), "project_id")
	require.NotContains(t, writer.Body.String(), `"project":""`)
}

func TestAntigravityGatewayService_ForwardGemini_ModelRateLimitTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	// 不需要真正调用上游，因为预检查会直接返回切换信号
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{},
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nil},
	}

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          2,
		Name:        "acc-gemini-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "proj",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
			},
		},
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"gemini-2.5-flash": map[string]any{
					"rate_limit_reset_at": futureResetAt,
				},
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, body, false)
	require.Nil(t, result, "ForwardGemini should not return result when model rate limited")
	require.NotNil(t, err, "ForwardGemini should return error")

	// 核心验证：错误应该是 UpstreamFailoverError，而不是普通 502 错误
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	// 非粘性会话请求，ForceCacheBilling 应为 false
	require.False(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be false for non-sticky session")
}

// TestAntigravityGatewayService_Forward_StickySessionForceCacheBilling
// 验证：粘性会话切换时，UpstreamFailoverError.ForceCacheBilling 应为 true
func TestAntigravityGatewayService_ForwardGemini_StickySessionForceCacheBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{},
		httpUpstream:  &httpUpstreamStub{resp: nil, err: nil},
	}

	// 设置模型限流：剩余时间 30 秒（> antigravityRateLimitThreshold 7s）
	futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	account := &Account{
		ID:          4,
		Name:        "acc-gemini-sticky-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "proj",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-2.5-flash",
			},
		},
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"gemini-2.5-flash": map[string]any{
					"rate_limit_reset_at": futureResetAt,
				},
			},
		},
	}

	// 传入 isStickySession = true
	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false, body, true)
	require.Nil(t, result, "ForwardGemini should not return result when model rate limited")
	require.NotNil(t, err, "ForwardGemini should return error")

	// 核心验证：粘性会话切换时，ForceCacheBilling 应为 true
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "error should be UpstreamFailoverError to trigger account switch")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.True(t, failoverErr.ForceCacheBilling, "ForceCacheBilling should be true for sticky session switch")
}

func TestAntigravityGatewayService_ForwardGemini_ClearsStickySessionOnGeminiRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", bytes.NewReader(body))
	c.Request = req

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)
	upstream := &httpUpstreamStub{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}}
	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{},
		httpUpstream:  upstream,
		accountRepo:   repo,
		cache:         cache,
	}

	account := &Account{
		ID:          44,
		Name:        "acc-gemini-runtime-rate-limited",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"expires_at":   time.Now().Add(time.Hour).Format(time.RFC3339),
			"project_id":   "proj",
		},
		Extra: map[string]any{
			"mixed_scheduling": true,
		},
	}

	result, err := svc.ForwardGemini(
		context.Background(),
		c,
		account,
		"gemini-3-flash-preview",
		"generateContent",
		false,
		body,
		true,
		WithForwardGeminiSession(77, "gemini:sticky-runtime"),
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Len(t, repo.modelRateLimitCalls, 2)
	require.Equal(t, "gemini-3-flash", repo.modelRateLimitCalls[0].modelKey)
	require.Equal(t, antigravityGeminiModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
	require.Len(t, cache.deleteCalls, 1)
	require.Equal(t, int64(77), cache.deleteCalls[0].groupID)
	require.Equal(t, "gemini:sticky-runtime", cache.deleteCalls[0].sessionHash)
}

// TestAntigravityGatewayService_Forward_BillsWithMappedModel
// 验证：Antigravity Claude 转发返回的计费模型使用映射后的模型
func TestAntigravityGatewayService_ForwardGemini_BillsWithMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	c.Request = req

	upstreamBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3}}}\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"req-bill-2"}},
		Body:       io.NopCloser(bytes.NewReader(upstreamBody)),
	}

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   &httpUpstreamStub{resp: resp},
	}

	const mappedModel = "gemini-3-pro-high"
	account := &Account{
		ID:          6,
		Name:        "acc-gemini-billing",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "proj",
			"model_mapping": map[string]any{
				"gemini-2.5-flash": mappedModel,
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, "gemini-2.5-flash", "generateContent", true, body, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gemini-2.5-flash", result.Model)
	require.Equal(t, mappedModel, result.UpstreamModel)
}

func TestAntigravityGatewayService_ForwardGemini_RetriesCorruptedThoughtSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
			{"role": "model", "parts": []map[string]any{{"text": "thinking", "thought": true, "thoughtSignature": "sig_bad_1"}}},
			{"role": "model", "parts": []map[string]any{{"functionCall": map[string]any{"name": "toolA", "args": map[string]any{"x": 1}}, "thoughtSignature": "sig_bad_2"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-3.1-pro-preview:streamGenerateContent", bytes.NewReader(body))
	c.Request = req

	firstRespBody := []byte(`{"response":{"error":{"code":400,"message":"Corrupted thought signature.","status":"INVALID_ARGUMENT"}}}`)
	secondRespBody := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3}}}\n\n")

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Request-Id": []string{"req-sig-1"},
				},
				Body: io.NopCloser(bytes.NewReader(firstRespBody)),
			},
			{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"X-Request-Id": []string{"req-sig-2"},
				},
				Body: io.NopCloser(bytes.NewReader(secondRespBody)),
			},
		},
	}

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}

	const originalModel = "gemini-3.1-pro-preview"
	const mappedModel = "gemini-3.1-pro-high"
	account := &Account{
		ID:          7,
		Name:        "acc-gemini-signature",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "proj",
			"model_mapping": map[string]any{
				originalModel: mappedModel,
			},
		},
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, originalModel, "streamGenerateContent", true, body, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, originalModel, result.Model)
	require.Equal(t, mappedModel, result.UpstreamModel)
	require.Len(t, upstream.requestBodies, 2, "signature error should trigger exactly one retry")

	firstReq := string(upstream.requestBodies[0])
	secondReq := string(upstream.requestBodies[1])
	require.Contains(t, firstReq, `"thoughtSignature":"sig_bad_1"`)
	require.Contains(t, firstReq, `"thoughtSignature":"sig_bad_2"`)
	require.Contains(t, secondReq, `"thoughtSignature":"skip_thought_signature_validator"`)
	require.NotContains(t, secondReq, `"thoughtSignature":"sig_bad_1"`)
	require.NotContains(t, secondReq, `"thoughtSignature":"sig_bad_2"`)

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)
	require.Equal(t, "signature_error", events[0].Kind)
}

func TestAntigravityGatewayService_ForwardGemini_SignatureRetryPropagatesFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
			{"role": "model", "parts": []map[string]any{{"text": "thinking", "thought": true, "thoughtSignature": "sig_bad_1"}}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-3.1-pro-preview:streamGenerateContent", bytes.NewReader(body))
	c.Request = req

	firstRespBody := []byte(`{"response":{"error":{"code":400,"message":"Corrupted thought signature.","status":"INVALID_ARGUMENT"}}}`)

	const originalModel = "gemini-3.1-pro-preview"
	const mappedModel = "gemini-3.1-pro-high"
	account := &Account{
		ID:          8,
		Name:        "acc-gemini-signature-failover",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "proj",
			"model_mapping": map[string]any{
				originalModel: mappedModel,
			},
		},
	}

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Request-Id": []string{"req-sig-failover-1"},
				},
				Body: io.NopCloser(bytes.NewReader(firstRespBody)),
			},
		},
		onCall: func(_ *http.Request, stub *queuedHTTPUpstreamStub) {
			if stub.callCount != 1 {
				return
			}
			futureResetAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
			account.Extra = map[string]any{
				modelRateLimitsKey: map[string]any{
					mappedModel: map[string]any{
						"rate_limit_reset_at": futureResetAt,
					},
				},
			}
		},
	}

	svc := &AntigravityGatewayService{
		settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}

	result, err := svc.ForwardGemini(context.Background(), c, account, originalModel, "streamGenerateContent", true, body, true)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "signature retry should propagate failover instead of falling back to the original 400")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.True(t, failoverErr.ForceCacheBilling)
	require.Len(t, upstream.requestBodies, 1, "retry should stop at preflight failover and not issue a second upstream request")

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Equal(t, "signature_error", events[0].Kind)
	require.Equal(t, "failover", events[1].Kind)
}

// TestStreamUpstreamResponse_UsageAndFirstToken
// 验证：usage 字段可被累积/覆盖更新，并且能记录首 token 时间
func TestHandleGeminiStreamingResponse_NormalComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}

	go func() {
		defer func() { _ = pw.Close() }()
		// 第一个 chunk（部分内容）
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3}}`)
		fmt.Fprintln(pw, "")
		// 第二个 chunk（最终内容+完整 usage）
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":8,"cachedContentTokenCount":2}}`)
		fmt.Fprintln(pw, "")
	}()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.clientDisconnect, "normal completion should not set clientDisconnect")
	require.NotNil(t, result.usage)
	// Gemini usage: promptTokenCount=10, candidatesTokenCount=8, cachedContentTokenCount=2
	// → InputTokens=10-2=8, OutputTokens=8, CacheReadInputTokens=2
	require.Equal(t, 8, result.usage.InputTokens)
	require.Equal(t, 8, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.CacheReadInputTokens)
	require.NotNil(t, result.firstTokenMs, "should record first token time")

	// 验证数据被透传到客户端
	body := rec.Body.String()
	require.Contains(t, body, "Hello")
	require.Contains(t, body, "world")
	// 不应包含错误事件
	require.NotContains(t, body, "event: error")
}

// TestHandleClaudeStreamingResponse_NormalComplete
// 验证：正常 Claude 流式转发（Gemini→Claude 转换），数据正确转换并输出
func TestHandleGeminiStreamingResponse_ThoughtsTokenCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}

	go func() {
		defer func() { _ = pw.Close() }()
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":50}}`)
		fmt.Fprintln(pw, "")
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":30,"thoughtsTokenCount":80,"cachedContentTokenCount":10}}`)
		fmt.Fprintln(pw, "")
	}()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	// promptTokenCount=100, cachedContentTokenCount=10 → InputTokens=90
	require.Equal(t, 90, result.usage.InputTokens)
	// candidatesTokenCount=30 + thoughtsTokenCount=80 → OutputTokens=110
	require.Equal(t, 110, result.usage.OutputTokens)
	require.Equal(t, 10, result.usage.CacheReadInputTokens)
}

// TestHandleClaudeStreamingResponse_ThoughtsTokenCount
// 验证：Gemini→Claude 流式转换时 thoughtsTokenCount 被计入 OutputTokens
func TestHandleGeminiStreamingResponse_ClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}

	go func() {
		defer func() { _ = pw.Close() }()
		fmt.Fprintln(pw, `data: {"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10}}`)
		fmt.Fprintln(pw, "")
	}()

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())
	_ = pr.Close()

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "write_failed")
}

// TestHandleGeminiStreamingResponse_ContextCanceled
// 验证：context 取消时不注入错误事件
func TestHandleGeminiStreamingResponse_ContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	})

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{StatusCode: http.StatusOK, Body: cancelReadCloser{}, Header: http.Header{}}

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.NotContains(t, rec.Body.String(), "event: error")
}

// TestHandleClaudeStreamingResponse_ClientDisconnect
// 验证：Claude 流式转发中客户端断开后继续 drain 上游
func TestAntigravityClientWriter(t *testing.T) {
	t.Run("normal write succeeds", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(c.Writer, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.True(t, ok)
		require.False(t, cw.Disconnected())
		require.Contains(t, rec.Body.String(), "hello")
	})

	t.Run("write failure marks disconnected", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.False(t, ok)
		require.True(t, cw.Disconnected())
	})

	t.Run("subsequent writes are no-op", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		cw.Write([]byte("first"))
		ok := cw.Fprintf("second %d", 2)
		require.False(t, ok)
		require.True(t, cw.Disconnected())
	})
}

// TestUnwrapV1InternalResponse 测试 unwrapV1InternalResponse 的各种输入场景
func TestUnwrapV1InternalResponse(t *testing.T) {
	svc := &AntigravityGatewayService{}

	// 构造 >50KB 的大型 JSON
	largePadding := strings.Repeat("x", 50*1024)
	largeInput := []byte(fmt.Sprintf(`{"response":{"id":"big","pad":"%s"}}`, largePadding))
	largeExpected := fmt.Sprintf(`{"id":"big","pad":"%s"}`, largePadding)

	tests := []struct {
		name     string
		input    []byte
		expected string
		wantErr  bool
	}{
		{
			name:     "正常 response 包装",
			input:    []byte(`{"response":{"id":"123","content":"hello"}}`),
			expected: `{"id":"123","content":"hello"}`,
		},
		{
			name:     "无 response 透传",
			input:    []byte(`{"id":"456"}`),
			expected: `{"id":"456"}`,
		},
		{
			name:     "空 JSON",
			input:    []byte(`{}`),
			expected: `{}`,
		},
		{
			name:     "response 为 null",
			input:    []byte(`{"response":null}`),
			expected: `null`,
		},
		{
			name:     "response 为基础类型 string",
			input:    []byte(`{"response":"hello"}`),
			expected: `"hello"`,
		},
		{
			name:     "非法 JSON",
			input:    []byte(`not json`),
			expected: `not json`,
		},
		{
			name:     "嵌套 response 只解一层",
			input:    []byte(`{"response":{"response":{"inner":true}}}`),
			expected: `{"response":{"inner":true}}`,
		},
		{
			name:     "大型 JSON >50KB",
			input:    largeInput,
			expected: largeExpected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.unwrapV1InternalResponse(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, strings.TrimSpace(string(got)))
		})
	}
}

// --- unwrapV1InternalResponse benchmark 对照组 ---

// unwrapV1InternalResponseOld 旧实现：Unmarshal+Marshal 双重开销（仅用于 benchmark 对照）
func unwrapV1InternalResponseOld(body []byte) ([]byte, error) {
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, err
	}
	if resp, ok := outer["response"]; ok {
		return json.Marshal(resp)
	}
	return body, nil
}

func BenchmarkUnwrapV1Internal_Old_Small(b *testing.B) {
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"hello world"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = unwrapV1InternalResponseOld(body)
	}
}

func BenchmarkUnwrapV1Internal_New_Small(b *testing.B) {
	body := []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"hello world"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}}`)
	svc := &AntigravityGatewayService{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.unwrapV1InternalResponse(body)
	}
}

func BenchmarkUnwrapV1Internal_Old_Large(b *testing.B) {
	body := generateLargeUnwrapJSON(10 * 1024) // ~10KB
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = unwrapV1InternalResponseOld(body)
	}
}

func BenchmarkUnwrapV1Internal_New_Large(b *testing.B) {
	body := generateLargeUnwrapJSON(10 * 1024) // ~10KB
	svc := &AntigravityGatewayService{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.unwrapV1InternalResponse(body)
	}
}

// generateLargeUnwrapJSON 生成指定最小大小的包含 response 包装的 JSON
func generateLargeUnwrapJSON(minSize int) []byte {
	parts := make([]map[string]string, 0)
	current := 0
	for current < minSize {
		text := fmt.Sprintf("这是第 %d 段内容，用于填充 JSON 到目标大小。", len(parts)+1)
		parts = append(parts, map[string]string{"text": text})
		current += len(text) + 20 // 估算 JSON 编码开销
	}
	inner := map[string]any{
		"candidates": []map[string]any{
			{"content": map[string]any{"parts": parts}},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     100,
			"candidatesTokenCount": 50,
		},
	}
	outer := map[string]any{"response": inner}
	b, _ := json.Marshal(outer)
	return b
}
