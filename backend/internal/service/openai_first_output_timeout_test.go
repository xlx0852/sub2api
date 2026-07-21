package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type blockingOpenAIResponseHeaderUpstream struct {
	canceled chan struct{}
	once     sync.Once
}

type firstOutputCloseTrackingBody struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (b *firstOutputCloseTrackingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.ReadCloser.Close()
}

func (u *blockingOpenAIResponseHeaderUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		u.once.Do(func() { close(u.canceled) })
		return nil, req.Context().Err()
	case <-time.After(1500 * time.Millisecond):
		return nil, errors.New("test upstream was not canceled before response headers")
	}
}

func (u *blockingOpenAIResponseHeaderUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, "", 0, 0)
}

func TestOpenAIForwardFirstOutputTimeoutIncludesResponseHeaderWait(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &blockingOpenAIResponseHeaderUpstream{canceled: make(chan struct{})}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIFirstOutputTimeoutSeconds: 1,
			MaxLineSize:                     defaultMaxLineSize,
		}},
		httpUpstream: upstream,
	}
	body := []byte(`{"model":"gpt-5.5","stream":true,"reasoning":{"effort":"low"},"input":"hello"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	account := &Account{
		ID: 1, Name: "oauth-test", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
	}

	started := time.Now()
	_, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "first_output_timeout")
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Less(t, time.Since(started), 1300*time.Millisecond)
	require.Empty(t, rec.Body.String())
	select {
	case <-upstream.canceled:
	default:
		t.Fatal("response-header timeout did not cancel the upstream request context")
	}
}

func TestOpenAINativeFirstOutputTimeoutDisabledPreservesSynchronousStream(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIFirstOutputTimeoutSeconds: 0,
		MaxLineSize:                     defaultMaxLineSize,
	}}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_disabled"}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_disabled","usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")))}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "response.completed")
}

func TestOpenAIFirstOutputTimeoutForReasoningEffort(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIFirstOutputTimeoutSeconds:           120,
		OpenAIHighEffortFirstOutputTimeoutSeconds: 300,
	}}}

	require.Equal(t, 120*time.Second, svc.openAIFirstOutputTimeout("low"))
	require.Equal(t, 300*time.Second, svc.openAIFirstOutputTimeout("high"))
	require.Equal(t, 300*time.Second, svc.openAIFirstOutputTimeout("xhigh"))
	require.Equal(t, 300*time.Second, svc.openAIFirstOutputTimeout("max"))
}

func TestOpenAIFirstOutputStageDefaultLimitIsIndependentFromScannerLimit(t *testing.T) {
	stage := newDefaultOpenAIFirstOutputStage()
	defer func() { require.NoError(t, stage.Close()) }()

	require.EqualValues(t, 8*1024*1024, stage.limit)
	require.Greater(t, stage.limit, int64(68106))
	require.Less(t, stage.limit, int64(defaultMaxLineSize))
}

func TestOpenAIFirstOutputEventQueueSizeBackpressuresGuardedStreams(t *testing.T) {
	require.Equal(t, 1, openAIFirstOutputEventQueueSize(true))
	require.Equal(t, 16, openAIFirstOutputEventQueueSize(false))
}

func TestOpenAIFirstOutputDynamicScannerLimitsOnlyWhileGuardIsActive(t *testing.T) {
	var guardActive atomic.Bool
	guardActive.Store(true)
	split := openAIFirstOutputDynamicScanLines(&guardActive)
	guardLimit := openAIFirstOutputStageMaxBytes + openAIFirstOutputScannerFramingAllowance
	undelimited := bytes.Repeat([]byte("x"), guardLimit)

	_, _, err := split(undelimited, false)
	require.ErrorIs(t, err, errOpenAIFirstOutputScannerLimit)

	guardActive.Store(false)
	advance, token, err := split(undelimited, false)
	require.NoError(t, err)
	require.Zero(t, advance)
	require.Nil(t, token)
}

func TestOpenAIFirstOutputStageOverflowIsAtomicAndCleanupRemovesSpool(t *testing.T) {
	stage := newOpenAIFirstOutputStage(70 * 1024)
	payload := bytes.Repeat([]byte("x"), 68*1024)
	n, err := stage.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	if runtime.GOOS == "windows" {
		require.Nil(t, stage.tempFile)
		require.Empty(t, stage.tempPath)
	} else {
		require.NotNil(t, stage.tempFile)
		require.NotEmpty(t, stage.tempPath)
		_, err = os.Stat(stage.tempPath)
		require.ErrorIs(t, err, os.ErrNotExist)
		stat, statErr := stage.tempFile.Stat()
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
	}

	n, err = stage.Write(bytes.Repeat([]byte("y"), 3*1024))
	require.Zero(t, n)
	require.ErrorIs(t, err, errOpenAIFirstOutputStageLimit)
	require.EqualValues(t, len(payload), stage.Buffered())
	path := stage.tempPath
	require.NoError(t, stage.Close())
	require.True(t, stage.closed)
	require.Nil(t, stage.tempFile)
	require.Empty(t, stage.tempPath)
	if path != "" {
		_, err = os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestOpenAIFirstOutputStageCommitCopiesSpoolAndRemovesTemp(t *testing.T) {
	stage := newOpenAIFirstOutputStage(80 * 1024)
	payload := bytes.Repeat([]byte("z"), 68*1024)
	_, err := stage.Write(payload)
	require.NoError(t, err)
	path := stage.tempPath
	if runtime.GOOS == "windows" {
		require.Empty(t, path)
		require.Nil(t, stage.tempFile)
	} else {
		require.NotEmpty(t, path)
		require.NotNil(t, stage.tempFile)
		_, statErr := os.Stat(path)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	}

	var downstream bytes.Buffer
	require.NoError(t, stage.CommitTo(&downstream))
	require.Equal(t, payload, downstream.Bytes())
	require.Zero(t, stage.Buffered())
	if path != "" {
		_, err = os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	require.NoError(t, stage.Close())
}

func TestOpenAIFirstOutputStageUnlinkFailurePermanentlyFallsBackToMemoryAndRetriesCleanup(t *testing.T) {
	stage := newDefaultOpenAIFirstOutputStage()
	stage.memoryOnly = false
	t.Cleanup(func() {
		stage.removeFile = os.Remove
		_ = stage.Close()
	})
	createCalls := 0
	stage.createTemp = func() (*os.File, error) {
		createCalls++
		return os.CreateTemp("", "sub2api-openai-first-output-fallback-*")
	}
	removeCalls := 0
	stage.removeFile = func(path string) error {
		removeCalls++
		if removeCalls <= 2 {
			return errors.New("forced remove failure")
		}
		return os.Remove(path)
	}

	payload := bytes.Repeat([]byte("m"), 68*1024)
	_, err := stage.Write(payload)
	require.NoError(t, err)
	require.True(t, stage.memoryOnly)
	require.Nil(t, stage.tempFile)
	require.NotEmpty(t, stage.tempPath)
	require.Equal(t, 1, createCalls)
	stat, statErr := os.Stat(stage.tempPath)
	require.NoError(t, statErr)
	require.Zero(t, stat.Size(), "failed-unlink fallback must never write plaintext to the named file")

	_, err = stage.WriteString("more")
	require.NoError(t, err)
	require.Equal(t, 1, createCalls, "memory-only fallback must not retry CreateTemp")
	path := stage.tempPath
	cleanupErr := stage.Close()
	require.ErrorContains(t, cleanupErr, "forced remove failure")
	require.Empty(t, stage.tempPath)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, stage.Close())
}

