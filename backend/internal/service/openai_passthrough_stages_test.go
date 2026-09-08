package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type passthroughStagesUpstream struct {
	httpUpstreamRecorder
	t       *testing.T
	tracker *FirstOutputStageTracker
}

func (u *passthroughStagesUpstream) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	require.Equal(u.t, firstOutputStageConnect, u.tracker.stage)
	trace := httptrace.ContextClientTrace(r.Context())
	require.NotNil(u.t, trace)
	require.NotNil(u.t, trace.WroteRequest)
	trace.WroteRequest(httptrace.WroteRequestInfo{})
	require.Equal(u.t, firstOutputStageUpstream, u.tracker.stage)
	time.Sleep(20 * time.Millisecond)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"status\":\"completed\"}}\n\n")), Request: r}, nil
}

func TestPassthroughFirstOutputStagesIncludeUpstreamWait(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := NewFirstOutputStageTracker(time.Now())
	ctx := WithFirstOutputStageTracker(context.Background(), tracker)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/responses", nil).WithContext(ctx)
	upstream := &passthroughStagesUpstream{t: t, tracker: tracker}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test"}, Extra: map[string]any{"openai_passthrough": true}}
	_, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.2","stream":true,"instructions":"test","input":"hi"}`))
	require.NoError(t, err)
	stages := tracker.Snapshot()
	require.NotNil(t, stages)
	require.True(t, *stages.UpstreamFirstOutputMs >= 20)
}
