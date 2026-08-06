package service

import "context"

// GeminiCliCodeAssistClient retained as retired no-op port.
type GeminiCliCodeAssistClient interface {
	LoadCodeAssist(ctx context.Context, accessToken, proxyURL string, req any) (any, error)
	OnboardUser(ctx context.Context, accessToken, proxyURL string, req any) (any, error)
}
