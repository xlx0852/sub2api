package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIForwardPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.openaiForwardStages()
	require.Len(t, stages, 7)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageAdmission,
		pipeline.StageSelect,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestGrokChatForwardPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.grokChatForwardStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestGrokForwardPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.grokForwardStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestResponsesChatFallbackPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.responsesChatFallbackStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestOpenAIChatForwardPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.openaiChatForwardStages()
	require.Len(t, stages, 7)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageAdmission,
		pipeline.StageSelect,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestOpenAIMessagesForwardPipelineStageOrder(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stages := svc.openaiMessagesForwardStages()
	require.Len(t, stages, 7)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageAdmission,
		pipeline.StageSelect,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestGrokForwardParseRejectsUnsupportedAccountType(t *testing.T) {
	svc := &OpenAIGatewayService{}
	st := &grokForwardState{body: []byte(`{"model":"grok"}`), originalModel: "grok"}
	ex := &pipeline.Exchange{Account: &Account{Platform: PlatformGrok, Type: AccountTypeUpstream}}
	ex.Set(grokForwardStateKey, st)
	err := svc.forwardGrokParse(ex)
	require.EqualError(t, err, "grok account type upstream is not supported by responses forwarding")
}

func TestResponsesChatFallbackParseRejectsMissingModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{}
	st := &responsesChatFallbackState{
		c:    c,
		body: []byte(`{"input":"hi"}`),
	}
	ex := &pipeline.Exchange{Inbound: st.body}
	ex.Set(responsesChatFallbackStateKey, st)
	err := svc.forwardResponsesChatFallbackParse(ex)
	require.EqualError(t, err, "missing model in request")
}

func TestKimiForwardRoutesResponsesToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"kimi-k3","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-kimi"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_kimi","object":"chat.completion","model":"k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true}}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID: 201, Name: "kimi", Platform: PlatformKimi, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "kimi-token", "base_url": "http://kimi.example/coding"},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://kimi.example/coding/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.Equal(t, "k3", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.False(t, result.Stream)
}
