// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

// VerificationCodeData represents verification code data
type VerificationCodeData struct {
	Code      string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time // absolute expiry; used to preserve remaining TTL when updating attempts
}

// PasswordResetTokenData represents password reset token data
type PasswordResetTokenData struct {
	Token     string
	CreatedAt time.Time
}
