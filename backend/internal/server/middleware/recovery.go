package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery converts panics into the project's standard JSON error envelope.
//
// It preserves Gin's broken-pipe handling by not attempting to write a response
// when the client connection is already gone.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, recovered any) {
		recoveredErr, _ := recovered.(error)

		if isBrokenPipe(recoveredErr) {
			if recoveredErr != nil {
				_ = c.Error(recoveredErr)
			}
			c.Abort()
			return
		}

		logRecoveryPanic(c, recovered)

		if c.Writer.Written() {
			c.Abort()
			return
		}

		response.ErrorWithDetails(
			c,
			http.StatusInternalServerError,
			infraerrors.UnknownMessage,
			infraerrors.UnknownReason,
			nil,
		)
		c.Abort()
	})
}

func logRecoveryPanic(c *gin.Context, recovered any) {
	fields := []zap.Field{
		zap.Any("panic", recovered),
		zap.String("stack", string(debug.Stack())),
	}
	if c != nil {
		if c.Request != nil {
			fields = append(fields,
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
			)
			if reqID, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok && reqID != "" {
				fields = append(fields, zap.String("request_id", reqID))
			}
		}
		if userID, exists := c.Get("user_id"); exists {
			fields = append(fields, zap.Any("user_id", userID))
		}
	}
	logger.L().Error("http.recovery_panic", fields...)
	fmt.Fprintf(gin.DefaultErrorWriter, "[Recovery] panic recovered: %v\n%s\n", recovered, debug.Stack())
}

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}

	var syscallErr *os.SyscallError
	if !errors.As(opErr.Err, &syscallErr) {
		return false
	}

	msg := strings.ToLower(syscallErr.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}
