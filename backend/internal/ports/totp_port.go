// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// TotpCache defines cache operations for TOTP service
type TotpCache interface {
	// Setup session methods
	GetSetupSession(ctx context.Context, userID int64) (*domain.TotpSetupSession, error)
	SetSetupSession(ctx context.Context, userID int64, session *domain.TotpSetupSession, ttl time.Duration) error
	DeleteSetupSession(ctx context.Context, userID int64) error

	// Login session methods (for 2FA login flow)
	GetLoginSession(ctx context.Context, tempToken string) (*domain.TotpLoginSession, error)
	SetLoginSession(ctx context.Context, tempToken string, session *domain.TotpLoginSession, ttl time.Duration) error
	DeleteLoginSession(ctx context.Context, tempToken string) error

	// Rate limiting
	IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error)
	GetVerifyAttempts(ctx context.Context, userID int64) (int, error)
	ClearVerifyAttempts(ctx context.Context, userID int64) error
}

// SecretEncryptor defines encryption operations for TOTP secrets
type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}
