// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"encoding/json"
)

const (
	ProviderOAuthFlowAuthorizationCode = "authorization_code"
	ProviderOAuthFlowDeviceCode        = "device_code"

	ProviderOAuthSessionPending    = "pending"
	ProviderOAuthSessionAuthorized = "authorized"
	ProviderOAuthSessionDenied     = "denied"
	ProviderOAuthSessionCancelled  = "cancelled"
)

// ProviderOAuthSession contains only the lifecycle state shared by provider
// login flows. Payload is provider-owned and never returned directly by a
// status handler.
type ProviderOAuthSession struct {
	ID                      string          `json:"id"`
	Provider                string          `json:"provider"`
	Flow                    string          `json:"flow"`
	Status                  string          `json:"status"`
	Version                 int64           `json:"version"`
	NextPollAtUnixMilli     int64           `json:"next_poll_at_unix_milli,omitempty"`
	PollLeaseID             string          `json:"poll_lease_id,omitempty"`
	PollLeaseUntilUnixMilli int64           `json:"poll_lease_until_unix_milli,omitempty"`
	ExpiresAtUnixMilli      int64           `json:"expires_at_unix_milli"`
	Payload                 json.RawMessage `json:"payload,omitempty"`
	Error                   string          `json:"error,omitempty"`
}

type ProviderOAuthSessionLease struct {
	Session *ProviderOAuthSession
	ID      string
	Held    bool
}
