package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

// modelCapacityRetryAfterSeconds 是模型级并发预算被拒后的建议重试间隔（秒）。
// 预算通常随请求完成/槽位过期很快释放，客户端退避一小段再重试即可。
const modelCapacityRetryAfterSeconds = 10

// modelCapacityLimitResponse 返回模型级并发预算已满时的降级响应三元组：
// 429 + rate_limit_error + 提示信息，并附 Retry-After 让 SDK 自动退避。
func modelCapacityLimitResponse(model string) (int, string, string) {
	if model == "" {
		model = "model"
	}
	return http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Model concurrency limit reached for %s, please retry later", model)
}

// writeModelCapacityLimitHeader 为模型预算被拒的响应写入 Retry-After 头。
func writeModelCapacityLimitHeader(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	c.Header("Retry-After", strconv.Itoa(modelCapacityRetryAfterSeconds))
}

func concurrencyErrorResponse(err error, slotType string) (int, string, string) {
	var waitQueueFullErr *WaitQueueFullError
	if errors.As(err, &waitQueueFullErr) {
		return http.StatusTooManyRequests, "rate_limit_error",
			"Too many pending requests, please retry later"
	}

	var concurrencyErr *ConcurrencyError
	if errors.As(err, &concurrencyErr) {
		if concurrencyErr.SlotType != "" {
			slotType = concurrencyErr.SlotType
		}
		return http.StatusTooManyRequests, "rate_limit_error",
			fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType)
	}

	if errors.Is(err, context.Canceled) {
		return statusClientClosedRequest, "api_error", "context canceled"
	}

	return http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later"
}
