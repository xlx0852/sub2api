package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	modelRateLimitsKey                 = domain.ModelRateLimitsKey
	antigravityGeminiModelRateLimitKey = domain.AntigravityGeminiModelRateLimitKey
	openAIImageGenerationRateLimitKey  = domain.OpenAIImageGenerationRateLimitKey
	anthropicFableRateLimitKey         = domain.AnthropicFableRateLimitKey
)

func isAnthropicFableModel(model string) bool {
	return domain.IsAnthropicFableModel(model)
}

func WithOpenAIImageGenerationIntent(ctx context.Context) context.Context {
	return domain.WithOpenAIImageGenerationIntent(ctx)
}

func OpenAIImageGenerationIntentFromContext(ctx context.Context) bool {
	return domain.OpenAIImageGenerationIntentFromContext(ctx)
}

func resolveFinalAntigravityModelKey(ctx context.Context, account *Account, requestedModel string) string {
	return domain.ResolveFinalAntigravityModelKey(ctx, account, requestedModel)
}

func isAntigravityGeminiModel(model string) bool {
	return domain.IsAntigravityGeminiModel(model)
}

func antigravityModelRateLimitKeys(model string) []string {
	return domain.AntigravityModelRateLimitKeys(model)
}
