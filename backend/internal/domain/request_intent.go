package domain

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type thinkingEnabledContextKey struct{}

// WithThinkingEnabledValue 将 thinking 开关写入 domain 自有 context key。
// service.WithThinkingEnabled 会同时写入 RequestMetadata / 旧 ctxkey 与本 key，
// 以便 Account 限流方法在不依赖 service 包的前提下读到同一语义。
func WithThinkingEnabledValue(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, thinkingEnabledContextKey{}, enabled)
}

// ThinkingEnabledFromContext 读取请求是否开启 thinking。
func ThinkingEnabledFromContext(ctx context.Context) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	if value, ok := ctx.Value(thinkingEnabledContextKey{}).(bool); ok {
		return value, true
	}
	if value, ok := ctx.Value(ctxkey.ThinkingEnabled).(bool); ok {
		return value, true
	}
	return false, false
}

func WithOpenAIImageGenerationIntent(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxkey.OpenAIImageGenerationIntent, true)
}

func OpenAIImageGenerationIntentFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, ok := ctx.Value(ctxkey.OpenAIImageGenerationIntent).(bool)
	return ok && enabled
}
