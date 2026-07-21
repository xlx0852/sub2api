package service

import (
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	OpenAIStreamStallReason = "upstream_stall_timeout"
	openAIDefaultStreamStallTimeoutSec = 300
)

// openAIStreamClock separates client keepalive arming from upstream progress.
type openAIStreamClock struct {
	stallTimeout time.Duration
	lastProgress atomic.Int64 // unix nano
	startedAt    time.Time
}

func newOpenAIStreamClock(stallTimeout time.Duration, now time.Time) *openAIStreamClock {
	clock := &openAIStreamClock{
		stallTimeout: stallTimeout,
		startedAt:    now,
	}
	clock.lastProgress.Store(now.UnixNano())
	return clock
}

func (c *openAIStreamClock) NoteUpstreamProgress(now time.Time) {
	if c == nil {
		return
	}
	c.lastProgress.Store(now.UnixNano())
}

// NoteClientKeepalive intentionally does not reset upstream progress.
func (c *openAIStreamClock) NoteClientKeepalive(_ time.Time) {}

func (c *openAIStreamClock) Stalled(now time.Time) bool {
	if c == nil || c.stallTimeout <= 0 {
		return false
	}
	last := time.Unix(0, c.lastProgress.Load())
	return now.Sub(last) >= c.stallTimeout
}

func openAIStreamStallTimeout(cfgSeconds int) time.Duration {
	if cfgSeconds < 0 {
		return 0
	}
	if cfgSeconds == 0 {
		// 0 means "use default enabled budget" for OpenAI Responses stall path.
		// Explicit disable is represented by negative config after validation clamp,
		// or by StreamDataIntervalTimeout == 0 in legacy path.
		return time.Duration(openAIDefaultStreamStallTimeoutSec) * time.Second
	}
	return time.Duration(cfgSeconds) * time.Second
}

// buildOpenAIResponsesIncompleteEvent builds a Responses-compatible incomplete terminal event.
func buildOpenAIResponsesIncompleteEvent(responseID, reason string) string {
	type incompleteDetails struct {
		Reason string `json:"reason"`
	}
	type responseBody struct {
		ID                string            `json:"id,omitempty"`
		Object            string            `json:"object"`
		Status            string            `json:"status"`
		IncompleteDetails incompleteDetails `json:"incomplete_details"`
	}
	type eventBody struct {
		Type           string       `json:"type"`
		SequenceNumber int          `json:"sequence_number"`
		Response       responseBody `json:"response"`
	}
	event := eventBody{
		Type:           "response.incomplete",
		SequenceNumber: 0,
		Response: responseBody{
			ID:     responseID,
			Object: "response",
			Status: "incomplete",
			IncompleteDetails: incompleteDetails{
				Reason: reason,
			},
		},
	}
	raw, err := json.Marshal(event)
	if err != nil {
		// Fallback minimal payload; reason is already constrained.
		return `data: {"type":"response.incomplete","sequence_number":0,"response":{"object":"response","status":"incomplete","incomplete_details":{"reason":` + strconv.Quote(reason) + `}}}` + "\n\n"
	}
	return "data: " + string(raw) + "\n\n"
}
