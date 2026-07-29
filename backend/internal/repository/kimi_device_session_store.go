package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type kimiDeviceSessionStore struct {
	provider service.ProviderOAuthSessionStore
}

func NewKimiDeviceSessionStore(rdb *redis.Client) service.KimiDeviceSessionStore {
	return &kimiDeviceSessionStore{provider: NewProviderOAuthSessionStore(rdb)}
}

func (s *kimiDeviceSessionStore) Create(ctx context.Context, session *service.KimiDeviceSession, ttl time.Duration) error {
	providerSession, err := kimiProviderOAuthSession(session)
	if err != nil {
		return err
	}
	return s.provider.Create(ctx, providerSession, ttl)
}

func (s *kimiDeviceSessionStore) Get(ctx context.Context, sessionID string) (*service.KimiDeviceSession, error) {
	providerSession, err := s.provider.Get(ctx, sessionID)
	if err != nil || providerSession == nil {
		return nil, err
	}
	return decodeKimiProviderOAuthSession(providerSession)
}

func (s *kimiDeviceSessionStore) AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*service.KimiDevicePollLease, error) {
	lease, err := s.provider.AcquirePollLease(ctx, sessionID, now, leaseTTL)
	if err != nil || lease == nil || lease.Session == nil {
		return nil, err
	}
	session, err := decodeKimiProviderOAuthSession(lease.Session)
	if err != nil {
		return nil, err
	}
	return &service.KimiDevicePollLease{Session: session, ProviderLease: lease, Held: lease.Held}, nil
}

func (s *kimiDeviceSessionStore) CommitPoll(ctx context.Context, lease *service.KimiDevicePollLease, updated *service.KimiDeviceSession) (bool, error) {
	if lease == nil || lease.ProviderLease == nil {
		return false, errors.New("Kimi device poll lease is invalid")
	}
	providerSession, err := kimiProviderOAuthSession(updated)
	if err != nil {
		return false, err
	}
	return s.provider.CommitPoll(ctx, lease.ProviderLease, providerSession)
}

func (s *kimiDeviceSessionStore) ConsumeAuthorized(ctx context.Context, sessionID string) (*service.KimiDeviceSession, error) {
	providerSession, err := s.provider.ConsumeAuthorized(ctx, sessionID)
	if err != nil || providerSession == nil {
		return nil, err
	}
	return decodeKimiProviderOAuthSession(providerSession)
}

func (s *kimiDeviceSessionStore) Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error) {
	return s.provider.Cancel(ctx, sessionID, tombstoneTTL)
}

func (s *kimiDeviceSessionStore) Delete(ctx context.Context, sessionID string) error {
	return s.provider.Delete(ctx, sessionID)
}

func kimiProviderOAuthSession(session *service.KimiDeviceSession) (*service.ProviderOAuthSession, error) {
	if session == nil {
		return nil, errors.New("Kimi device session is nil")
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	return &service.ProviderOAuthSession{
		ID:                  session.ID,
		Provider:            service.PlatformKimi,
		Flow:                service.ProviderOAuthFlowDeviceCode,
		Status:              session.Status,
		NextPollAtUnixMilli: session.NextPollAt.UnixMilli(),
		ExpiresAtUnixMilli:  session.ExpiresAt.UnixMilli(),
		Payload:             payload,
		Error:               session.Error,
	}, nil
}

func decodeKimiProviderOAuthSession(provider *service.ProviderOAuthSession) (*service.KimiDeviceSession, error) {
	if provider == nil {
		return nil, nil
	}
	if provider.Status == service.ProviderOAuthSessionCancelled {
		return &service.KimiDeviceSession{
			ID:        provider.ID,
			Status:    provider.Status,
			ExpiresAt: time.UnixMilli(provider.ExpiresAtUnixMilli),
		}, nil
	}
	var session service.KimiDeviceSession
	if len(provider.Payload) == 0 {
		return nil, errors.New("Kimi device session payload is missing")
	}
	if err := json.Unmarshal(provider.Payload, &session); err != nil {
		return nil, err
	}
	session.ID = provider.ID
	session.Status = provider.Status
	session.NextPollAt = time.UnixMilli(provider.NextPollAtUnixMilli)
	session.ExpiresAt = time.UnixMilli(provider.ExpiresAtUnixMilli)
	session.Error = provider.Error
	return &session, nil
}
