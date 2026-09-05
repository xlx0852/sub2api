//go:build unit

package service

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIStreamClock struct {
	stallTimeout time.Duration
	lastProgress atomic.Int64
	startedAt    time.Time
}

func newOpenAIStreamClock(stallTimeout time.Duration, now time.Time) *openAIStreamClock {
	clock := &openAIStreamClock{stallTimeout: stallTimeout, startedAt: now}
	clock.lastProgress.Store(now.UnixNano())
	return clock
}

func (c *openAIStreamClock) NoteUpstreamProgress(now time.Time) {
	if c != nil {
		c.lastProgress.Store(now.UnixNano())
	}
}

func (c *openAIStreamClock) NoteClientKeepalive(_ time.Time) {}

func (c *openAIStreamClock) Stalled(now time.Time) bool {
	if c == nil || c.stallTimeout <= 0 {
		return false
	}
	return now.Sub(time.Unix(0, c.lastProgress.Load())) >= c.stallTimeout
}

func TestOpenAIStreamClockKeepaliveDoesNotResetStall(t *testing.T) {
	now := time.Now()
	clock := newOpenAIStreamClock(100*time.Millisecond, now)
	clock.NoteClientKeepalive(now.Add(50 * time.Millisecond))
	require.False(t, clock.Stalled(now.Add(50*time.Millisecond)))
	require.True(t, clock.Stalled(now.Add(150*time.Millisecond)))

	clock.NoteUpstreamProgress(now.Add(160 * time.Millisecond))
	require.False(t, clock.Stalled(now.Add(200*time.Millisecond)))
}

func TestBuildOpenAIResponsesIncompleteEvent(t *testing.T) {
	payload := buildOpenAIResponsesIncompleteEvent("resp_1", OpenAIStreamStallReason)
	require.True(t, strings.HasPrefix(payload, "data: "))
	require.Contains(t, payload, `"type":"response.incomplete"`)
	require.Contains(t, payload, `"status":"incomplete"`)
	require.Contains(t, payload, OpenAIStreamStallReason)
	require.NotContains(t, payload, `"status":"completed"`)
}
