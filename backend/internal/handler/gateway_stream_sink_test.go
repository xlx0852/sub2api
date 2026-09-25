package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/stream"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsStreamSinkEmitsAndFlushes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	sink := newChatCompletionsStreamSink(ctx)

	require.NoError(t, sink.Emit(stream.Event{Kind: "chat.completions.sse", Data: []byte("data: hello\\n\\n")}))
	require.NoError(t, sink.Flush())
	require.Equal(t, "data: hello\\n\\n", recorder.Body.String())
}
