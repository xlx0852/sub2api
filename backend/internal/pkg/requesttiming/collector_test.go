package requesttiming

import (
	"context"
	"testing"
	"time"
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
