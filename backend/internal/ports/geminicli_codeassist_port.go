// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
)

// GeminiCliCodeAssistClient retained as retired no-op port.
type GeminiCliCodeAssistClient interface {
	LoadCodeAssist(ctx context.Context, accessToken, proxyURL string, req any) (any, error)
	OnboardUser(ctx context.Context, accessToken, proxyURL string, req any) (any, error)
}
