package transport

import "net/http"

// Upstream 是管道 Transport 阶段对上游 HTTP 往返的最小端口。
// 现有 ports.HTTPUpstream 在平台迁移时作为实现接入，避免管道依赖 service。
type Upstream interface {
	Do(req *http.Request) (*http.Response, error)
}
