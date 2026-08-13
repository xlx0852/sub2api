// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

const (
	IdempotencyStatusProcessing      = "processing"
	IdempotencyStatusSucceeded       = "succeeded"
	IdempotencyStatusFailedRetryable = "failed_retryable"
)

type IdempotencyRecord struct {
	ID                 int64
	Scope              string
	IdempotencyKeyHash string
	RequestFingerprint string
	Status             string
	ResponseStatus     *int
	ResponseBody       *string
	ErrorReason        *string
	LockedUntil        *time.Time
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
