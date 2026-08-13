// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// API Key status constants
const (
	StatusAPIKeyActive         = "active"
	StatusAPIKeyDisabled       = "disabled"
	StatusAPIKeyQuotaExhausted = "quota_exhausted"
	StatusAPIKeyExpired        = "expired"
)

// Rate limit window durations
const (
	RateLimitWindow5h = 5 * time.Hour
	RateLimitWindow1d = 24 * time.Hour
	RateLimitWindow7d = 7 * 24 * time.Hour
)

// IsWindowExpired returns true if the window starting at windowStart has exceeded the given duration.
// A nil windowStart is treated as expired — no initialized window means any accumulated usage is stale.
func IsWindowExpired(windowStart *time.Time, duration time.Duration) bool {
	return windowStart == nil || time.Since(*windowStart) >= duration
}

// APIKeyListFilters holds optional filtering parameters for listing API keys.
type APIKeyListFilters struct {
	Search  string
	Status  string
	GroupID *int64 // nil=不筛选, 0=无分组, >0=指定分组
}

var ErrAPIKeyNotFound = infraerrors.NotFound("API_KEY_NOT_FOUND", "api key not found")

var ErrAPIKeyExists = infraerrors.Conflict("API_KEY_EXISTS", "api key already exists")

// APIKeyRateLimitData holds rate limit usage and window state for an API key.
type APIKeyRateLimitData struct {
	Usage5h       float64
	Usage1d       float64
	Usage7d       float64
	Window5hStart *time.Time
	Window1dStart *time.Time
	Window7dStart *time.Time
}

// EffectiveUsage5h returns the 5h window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage5h() float64 {
	if IsWindowExpired(d.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return d.Usage5h
}

// EffectiveUsage1d returns the 1d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage1d() float64 {
	if IsWindowExpired(d.Window1dStart, RateLimitWindow1d) {
		return 0
	}
	return d.Usage1d
}

// EffectiveUsage7d returns the 7d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage7d() float64 {
	if IsWindowExpired(d.Window7dStart, RateLimitWindow7d) {
		return 0
	}
	return d.Usage7d
}

// APIKeyQuotaUsageState captures the latest quota fields after an atomic quota update.
// It is intentionally small so repositories can return it from a single SQL statement.
type APIKeyQuotaUsageState struct {
	QuotaUsed float64
	Quota     float64
	Key       string
	Status    string
}
