package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/app/gateway/stream"
	"github.com/gin-gonic/gin"
)

// chatCompletionsStreamSink 把 transport-neutral 事件编码结果交给 Gin。
// service 只依赖 stream.Sink，不再直接调用 ResponseWriter/Flush。
type chatCompletionsStreamSink struct {
	c *gin.Context
}

func newChatCompletionsStreamSink(c *gin.Context) stream.Sink {
	if c == nil {
		return nil
	}
	return &chatCompletionsStreamSink{c: c}
}

func (s *chatCompletionsStreamSink) Emit(event stream.Event) error {
	if s == nil || s.c == nil {
		return nil
	}
	_, err := s.c.Writer.Write(event.Data)
	return err
}

func (s *chatCompletionsStreamSink) Flush() error {
	if s == nil || s.c == nil {
		return nil
	}
	s.c.Writer.Flush()
	return nil
}
