package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

func submitUsageRecordTask(parent context.Context, task service.UsageRecordTask, durable bool, pool *service.UsageRecordWorkerPool, component, prefix string) {
	if task == nil {
		return
	}
	task = wrapUsageRecordTaskContext(parent, task)
	if durable {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		defer recoverUsageRecordTaskPanic(component, prefix+".durable_usage_handoff_panic_recovered")()
		task(ctx)
		return
	}
	if pool != nil {
		pool.Submit(task)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer recoverUsageRecordTaskPanic(component, prefix+".usage_record_task_panic_recovered")()
	task(ctx)
}

func recoverUsageRecordTaskPanic(component, message string) func() {
	return func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(zap.String("component", component), zap.Any("panic", recovered)).Error(message)
		}
	}
}

func usageRecordContext(parent context.Context, base context.Context) context.Context {
	if base == nil {
		base = context.Background()
	}
	if parent == nil {
		return base
	}
	if clientRequestID, _ := parent.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
		base = context.WithValue(base, ctxkey.ClientRequestID, strings.TrimSpace(clientRequestID))
	}
	if requestID, _ := parent.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
		base = context.WithValue(base, ctxkey.RequestID, strings.TrimSpace(requestID))
	}
	if groupID := service.UsageSettlementGroupIDFromContext(parent); groupID != nil {
		base = service.WithUsageSettlementGroupID(base, groupID)
	}
	return base
}

func wrapUsageRecordTaskContext(parent context.Context, task service.UsageRecordTask) service.UsageRecordTask {
	if task == nil {
		return nil
	}
	return func(ctx context.Context) { task(usageRecordContext(parent, ctx)) }
}
