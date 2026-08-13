// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrPromoCodeNotFound = infraerrors.NotFound("PROMO_CODE_NOT_FOUND", "promo code not found")
