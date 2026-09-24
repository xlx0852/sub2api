package requesttiming

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestCollectorSnapshotKeepsUncollectedFieldsNil(t *testing.T) {
	start := time.Now()
	c := New(start)
	out := c.Snapshot(start.Add(time.Second))
	if out == nil {
		t.Fatal("snapshot must not be nil")
	}
	if out.BodyBytes != nil || out.BodyFirstWaitMs != nil || out.UpstreamAttempts != nil || out.Outcome != "" {
		t.Fatalf("uncollected fields must stay nil: %+v", out)
	}
	if out.HandlerTotalMs == nil || *out.HandlerTotalMs != 1000 {
		t.Fatalf("handler total = %v, want 1000", out.HandlerTotalMs)
	}
}

func TestCollectorRecordsBodyAndUpstreamStages(t *testing.T) {
	start := time.Now()
	c := New(start)
	c.BodyRead(5*time.Millisecond, 100, false)
	c.BodyRead(3*time.Millisecond, 200, true)
	c.SetAPIKeyAuthMs(7 * time.Millisecond)
	c.SetUserQueueMs(2 * time.Millisecond)
	c.SetAccountSelectMs(11 * time.Millisecond)
	c.SetAccountQueueMs(13 * time.Millisecond)
	c.Attempt()
	c.Attempt()
	c.SetConnectionReused(true)
	c.MarkUpstreamConnect()
	c.MarkUpstreamWrite()
	c.MarkUpstreamFirstByte()
	c.MarkUpstreamHeaders()
	c.SetStreamComplete(true)
	c.MarkUpstreamStream()
	c.SetOutcome("success")
	c.MarkHandlerEnd(start.Add(2 * time.Second))

	out := c.Snapshot(time.Now())
	if out.BodyBytes == nil || *out.BodyBytes != 300 {
		t.Fatalf("body bytes = %v, want 300", out.BodyBytes)
	}
	if out.BodyComplete == nil || !*out.BodyComplete {
		t.Fatal("body complete must be true after EOF")
	}
	if out.BodyReadMs == nil || *out.BodyReadMs != 8 {
		t.Fatalf("body read ms = %v, want 8", out.BodyReadMs)
	}
	if out.HandlerBodyReadMs == nil || *out.HandlerBodyReadMs != 8 {
		t.Fatalf("handler body read must mirror body read")
	}
	if out.UpstreamAttempts == nil || *out.UpstreamAttempts != 2 {
		t.Fatalf("attempts = %v, want 2", out.UpstreamAttempts)
	}
	if out.ConnectionReused == nil || !*out.ConnectionReused {
		t.Fatal("connection reused must be recorded")
	}
	if out.APIKeyAuthMs == nil || *out.APIKeyAuthMs != 7 {
		t.Fatalf("api key auth ms = %v, want 7", out.APIKeyAuthMs)
	}
	if out.UpstreamFirstByteMs == nil || *out.UpstreamFirstByteMs < 0 {
		t.Fatalf("first byte must be recorded: %v", out.UpstreamFirstByteMs)
	}
	if out.ResponseStreamComplete == nil || !*out.ResponseStreamComplete {
		t.Fatal("stream complete must be recorded")
	}
	if out.Outcome != "success" {
		t.Fatalf("outcome = %q", out.Outcome)
	}
	if out.HandlerTotalMs == nil || *out.HandlerTotalMs != 2000 {
		t.Fatalf("handler total = %v, want 2000", out.HandlerTotalMs)
	}
}

func TestCollectorContextRoundTripAndNilSafety(t *testing.T) {
	var nilCollector *Collector
	nilCollector.BodyRead(time.Millisecond, 1, true)
	nilCollector.Attempt()
	nilCollector.SetOutcome("failed")
	if nilCollector.Snapshot(time.Now()) != nil {
		t.Fatal("nil collector snapshot must be nil")
	}
	if FromContext(context.Background()) != nil {
		t.Fatal("missing collector must be nil")
	}
	c := New(time.Now())
	ctx := WithCollector(context.Background(), c)
	if FromContext(ctx) != c {
		t.Fatal("collector must round-trip through context")
	}
}

func TestCollectorEmitsWaterfallSegments(t *testing.T) {
	start := time.Now()
	c := New(start)
	c.BodyRead(2*time.Millisecond, 10, false)
	c.BodyRead(time.Millisecond, 20, true)
	c.SetAPIKeyAuthMs(5 * time.Millisecond)
	c.SetSecurityAuditMs(3 * time.Millisecond)
	c.SetAccountSelectMs(4 * time.Millisecond)
	c.Attempt()
	attempt := c.AttemptCount()
	if attempt != 1 {
		t.Fatalf("attempt = %d", attempt)
	}
	c.AddAttemptSegment("upstream_connect", attempt, c.UpstreamAttemptStart(attempt), start.Add(30*time.Millisecond))
	c.SetOutcome("success")

	out := c.Snapshot(time.Now())
	if out == nil || len(out.Segments) == 0 {
		t.Fatal("segments must be recorded")
	}
	keys := map[string]domain.TimingSegment{}
	for _, segment := range out.Segments {
		keys[segment.Key] = segment
	}
	for _, want := range []string{"body_wait", "body_receive", "api_key_auth", "security_audit", "account_select", "upstream_connect"} {
		if _, ok := keys[want]; !ok {
			t.Fatalf("missing segment %q in %+v", want, out.Segments)
		}
	}
	if keys["body_wait"].Lane != "client" || keys["api_key_auth"].Lane != "gateway" || keys["upstream_connect"].Lane != "upstream" {
		t.Fatalf("unexpected lanes: %+v", keys)
	}
	if keys["upstream_connect"].Attempt != 1 {
		t.Fatalf("attempt index missing: %+v", keys["upstream_connect"])
	}
	if keys["body_wait"].Start != 0 || keys["body_wait"].End < 0 {
		t.Fatalf("body_wait must start at 0: %+v", keys["body_wait"])
	}
	// 区间不得倒挂
	for _, segment := range out.Segments {
		if segment.End < segment.Start {
			t.Fatalf("segment %q end < start: %+v", segment.Key, segment)
		}
	}
}
