//go:build unit

package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
