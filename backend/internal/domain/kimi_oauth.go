// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

type KimiDevicePollLease struct {
	Session       *KimiDeviceSession
	ProviderLease *ProviderOAuthSessionLease
	Held          bool
}

type KimiDeviceSession struct {
	ID                      string              `json:"id"`
	Status                  string              `json:"status"`
	DeviceCode              string              `json:"device_code"`
	UserCode                string              `json:"user_code"`
	VerificationURI         string              `json:"verification_uri"`
	VerificationURIComplete string              `json:"verification_uri_complete"`
	ProxyURL                string              `json:"proxy_url,omitempty"`
	ProxyID                 *int64              `json:"proxy_id,omitempty"`
	DeviceID                string              `json:"device_id"`
	DeviceName              string              `json:"device_name"`
	DeviceModel             string              `json:"device_model"`
	IntervalSeconds         int64               `json:"interval_seconds"`
	NextPollAt              time.Time           `json:"next_poll_at"`
	ExpiresAt               time.Time           `json:"expires_at"`
	Token                   *KimiOAuthTokenInfo `json:"token,omitempty"`
	Error                   string              `json:"error,omitempty"`
}

type KimiOAuthTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name"`
	DeviceModel  string `json:"device_model"`
	// UserID is the durable Kimi subject (JWT user_id/sub). Used as the OAuth
	// merge identity because Kimi device tokens do not include an email claim.
	UserID  string `json:"user_id,omitempty"`
	ProxyID *int64 `json:"proxy_id,omitempty"`
}
