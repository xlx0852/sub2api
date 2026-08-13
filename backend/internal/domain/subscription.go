// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrSubscriptionNotFound = infraerrors.NotFound("SUBSCRIPTION_NOT_FOUND", "subscription not found")

var ErrSubscriptionAlreadyExists = infraerrors.Conflict("SUBSCRIPTION_ALREADY_EXISTS", "subscription already exists for this user and group")

var ErrSubscriptionRestoreConflict = infraerrors.Conflict("SUBSCRIPTION_RESTORE_CONFLICT", "subscription already exists for this user and group")

var ErrSubscriptionNilInput = infraerrors.BadRequest("SUBSCRIPTION_NIL_INPUT", "subscription input cannot be nil")
