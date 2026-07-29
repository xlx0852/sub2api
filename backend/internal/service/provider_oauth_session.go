package service

import (
	"context"
	"encoding/json"
	"time"
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

type ProviderOAuthSessionStore interface {
	Create(ctx context.Context, session *ProviderOAuthSession, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (*ProviderOAuthSession, error)
	AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*ProviderOAuthSessionLease, error)
	CommitPoll(ctx context.Context, lease *ProviderOAuthSessionLease, updated *ProviderOAuthSession) (bool, error)
	Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error)
	ConsumeAuthorized(ctx context.Context, sessionID string) (*ProviderOAuthSession, error)
	Delete(ctx context.Context, sessionID string) error
}
