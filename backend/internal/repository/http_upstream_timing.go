package repository

import (
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// withUpstreamTiming 给上游请求挂上 httptrace，把连接、写入、首字节、响应头与
// 流传输写成瀑布图的阶段区间（lane=upstream，带尝试序号）。
// 未接入采集器时原样返回，零开销。
func withUpstreamTiming(req *http.Request) *http.Request {
	if req == nil {
		return req
	}
	collector := requesttiming.FromContext(req.Context())
	if collector == nil {
		return req
	}
	collector.Attempt()
	attempt := collector.AttemptCount()
	doStart := time.Now()
	last := doStart
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			now := time.Now()
			collector.SetConnectionReused(info.Reused)
			collector.MarkUpstreamConnect()
			collector.AddAttemptSegment("upstream_connect", attempt, doStart, now)
			last = now
		},
		WroteHeaders: func() {
			collector.MarkUpstreamWroteHeaders()
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			now := time.Now()
			collector.MarkUpstreamWrite()
			collector.AddAttemptSegment("upstream_write", attempt, last, now)
			last = now
		},
		GotFirstResponseByte: func() {
			now := time.Now()
			collector.MarkUpstreamFirstByte()
			collector.AddAttemptSegment("upstream_first_byte", attempt, last, now)
			last = now
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

// trackUpstreamResponseBody 包装响应体：响应头就绪与流传输结束各记一段，
// 读到 EOF 视为响应流完整，其余情况（错误、提前关闭）标记为不完整。
func trackUpstreamResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	collector := requesttiming.FromContext(resp.Request.Context())
	if collector == nil {
		return
	}
	headersAt := time.Now()
	collector.MarkUpstreamHeaders()
	attempt := collector.AttemptCount()
	collector.AddAttemptSegment("upstream_response_headers", attempt, collector.UpstreamAttemptStart(attempt), headersAt)
	resp.Body = &timedUpstreamBody{ReadCloser: resp.Body, collector: collector, attempt: attempt, startedAt: headersAt}
}

type timedUpstreamBody struct {
	io.ReadCloser
	collector *requesttiming.Collector
	attempt   int
	startedAt time.Time
	finished  bool
}

func (b *timedUpstreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !b.finished {
		b.finished = true
		now := time.Now()
		b.collector.SetStreamComplete(err == io.EOF)
		b.collector.MarkUpstreamStream()
		b.collector.AddAttemptSegment("upstream_stream", b.attempt, b.startedAt, now)
	}
	return n, err
}

func (b *timedUpstreamBody) Close() error {
	if !b.finished {
		b.finished = true
		now := time.Now()
		b.collector.SetStreamComplete(false)
		b.collector.MarkUpstreamStream()
		b.collector.AddAttemptSegment("upstream_stream", b.attempt, b.startedAt, now)
	}
	return b.ReadCloser.Close()
}
