// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrRedeemCodeNotFound = infraerrors.NotFound("REDEEM_CODE_NOT_FOUND", "redeem code not found")

var ErrRedeemCodeUsed = infraerrors.Conflict("REDEEM_CODE_USED", "redeem code already used")

type NullableTimeUpdate struct {
	Set   bool
	Value *time.Time
}

type NullableInt64Update struct {
	Set   bool
	Value *int64
}

type RedeemCodeBatchUpdateFields struct {
	Status    *string
	ExpiresAt NullableTimeUpdate
	Notes     *string
	GroupID   NullableInt64Update

	// Core fields are intentionally modeled only so service validation can
	// reject payloads that try to mutate redemption value semantics in bulk.
	Type  *string
	Value *float64
}

func (f RedeemCodeBatchUpdateFields) HasChanges() bool {
	return f.Status != nil ||
		f.ExpiresAt.Set ||
		f.Notes != nil ||
		f.GroupID.Set ||
		f.Type != nil ||
		f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) HasCoreFieldChanges() bool {
	return f.Type != nil || f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) TouchesUsedSensitiveFields() bool {
	return f.Status != nil || f.ExpiresAt.Set || f.GroupID.Set
}
