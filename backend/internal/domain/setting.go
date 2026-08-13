// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type Setting struct {
	ID        int64
	Key       string
	Value     string
	UpdatedAt time.Time
}

var ErrSettingNotFound = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
