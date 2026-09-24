package repository

import (
	"io"
	"net/http"
	"net/http/httptrace"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// withUpstreamTiming 给上游请求挂上 httptrace，把连接、写入、首字节时间点写进
// 请求级采集器（requesttiming.Collector）。未接入采集器时原样返回，零开销路径。
//
// 说明：Do 每执行一次记为一次「出口尝试」；连接复用来自 GotConn.Reused。
func withUpstreamTiming(req *http.Request) *http.Request {
	if req == nil {
		return req
	}
	collector := requesttiming.FromContext(req.Context())
	if collector == nil {
		return req
	}
	collector.Attempt()
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			collector.SetConnectionReused(info.Reused)
			collector.MarkUpstreamConnect()
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			collector.MarkUpstreamWrite()
		},
		GotFirstResponseByte: func() {
			collector.MarkUpstreamFirstByte()
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

// trackUpstreamResponseBody 包装响应体：读到 EOF 视为响应流完整，其余情况
// （错误、提前关闭）标记为不完整，并记录流传输结束时间。
func trackUpstreamResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	collector := requesttiming.FromContext(resp.Request.Context())
	if collector == nil {
		return
	}
	collector.MarkUpstreamHeaders()
	resp.Body = &timedUpstreamBody{ReadCloser: resp.Body, collector: collector}
}

type timedUpstreamBody struct {
	io.ReadCloser
	collector *requesttiming.Collector
	finished  bool
}

func (b *timedUpstreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !b.finished {
		b.finished = true
		b.collector.SetStreamComplete(err == io.EOF)
		b.collector.MarkUpstreamStream()
	}
	return n, err
}

func (b *timedUpstreamBody) Close() error {
	if !b.finished {
		b.finished = true
		b.collector.SetStreamComplete(false)
		b.collector.MarkUpstreamStream()
	}
	return b.ReadCloser.Close()
}
