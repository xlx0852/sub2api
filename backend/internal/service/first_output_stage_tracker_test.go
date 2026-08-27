package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFirstOutputStageTrackerReconcilesStages(t *testing.T) {
	start := time.Unix(100, 0)
	tracker := NewFirstOutputStageTracker(start)
	tracker.transition(firstOutputStageConnect, start.Add(10*time.Millisecond), false)
	tracker.transition(firstOutputStageUpstream, start.Add(15*time.Millisecond), false)
	tracker.transition(firstOutputStageWrite, start.Add(45*time.Millisecond), false)
	tracker.finish(start.Add(48 * time.Millisecond))

	stages := tracker.Snapshot()
	require.NotNil(t, stages)
	require.Equal(t, 10, *stages.GatewayPreUpstreamMs)
	require.Equal(t, 5, *stages.UpstreamConnectMs)
	require.Equal(t, 30, *stages.UpstreamFirstOutputMs)
	require.Equal(t, 0, *stages.RetryFailoverMs)
	require.Equal(t, 3, *stages.GatewayFirstOutputWriteMs)
	require.Equal(t, 48, *stages.GatewayFirstOutputTotalMs)
}

func TestFirstOutputStageTrackerMovesFailedUpstreamWaitToRetry(t *testing.T) {
	start := time.Unix(100, 0)
	tracker := NewFirstOutputStageTracker(start)
	tracker.transition(firstOutputStageConnect, start.Add(10*time.Millisecond), false)
	tracker.transition(firstOutputStageUpstream, start.Add(15*time.Millisecond), false)
	tracker.transition(firstOutputStageRetry, start.Add(35*time.Millisecond), true)
	tracker.transition(firstOutputStageConnect, start.Add(40*time.Millisecond), false)
	tracker.transition(firstOutputStageUpstream, start.Add(44*time.Millisecond), false)
	tracker.transition(firstOutputStageWrite, start.Add(74*time.Millisecond), false)
	tracker.finish(start.Add(76 * time.Millisecond))

	stages := tracker.Snapshot()
	require.Equal(t, 30, *stages.UpstreamFirstOutputMs)
	require.Equal(t, 25, *stages.RetryFailoverMs)
	require.Equal(t, 76, *stages.GatewayFirstOutputTotalMs)
}

func TestFirstOutputStageTrackerReturnsNilBeforeFirstOutputFlush(t *testing.T) {
	tracker := NewFirstOutputStageTracker(time.Now())
	require.Nil(t, tracker.Snapshot())
}

func TestFirstOutputStageTrackerFinishesThroughResponsesSSERelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	tracker := NewFirstOutputStageTracker(time.Now())
	ctx := WithFirstOutputStageTracker(c.Request.Context(), tracker)
	upstream := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_stage","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(upstream))}
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}

	result, err := svc.handleStreamingResponse(ctx, resp, c, &Account{ID: 1}, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result.firstTokenMs)
	require.NotNil(t, tracker.Snapshot())
}
