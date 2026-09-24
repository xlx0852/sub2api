package middleware

import (
	"io"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// RequestTiming 为单次请求建立耗时采集器：
//   - 请求开始时创建采集器并写入 context（上游 httpUpstream.Do 依赖它挂 httptrace）；
//   - 包装 c.Request.Body，统计首读等待、接收区间、读体阻塞累计、字节数与完整性；
//   - 请求结束（Handler 返回）时记录 Handler 总耗时。
//
// 采集失败或未接入时不影响请求；本中间件不读取、不缓存请求体，只做计数与计时。
func RequestTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil || c.Request == nil {
			c.Next()
			return
		}
		collector := requesttiming.New(time.Now())
		c.Request = c.Request.WithContext(requesttiming.WithCollector(c.Request.Context(), collector))
		if c.Request.Body != nil {
			c.Request.Body = &timedRequestBody{ReadCloser: c.Request.Body, collector: collector}
		}
		c.Next()
		collector.MarkHandlerEnd(time.Now())
	}
}

type timedRequestBody struct {
	io.ReadCloser
	collector *requesttiming.Collector
}

func (b *timedRequestBody) Read(p []byte) (int, error) {
	start := time.Now()
	n, err := b.ReadCloser.Read(p)
	blocked := time.Since(start)
	eof := err == io.EOF
	b.collector.BodyRead(blocked, n, eof)
	return n, err
}
