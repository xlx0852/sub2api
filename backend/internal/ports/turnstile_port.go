// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// TurnstileVerifier 验证 Turnstile token 的接口
type TurnstileVerifier interface {
	VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*domain.TurnstileVerifyResponse, error)
}
