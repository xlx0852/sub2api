package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type kimiOAuthClientStub struct {
	device  *kimi.DeviceCodeResponse
	polls   []*kimi.TokenResponse
	refresh *kimi.TokenResponse
	headers []kimi.DeviceHeaders
	proxies []string
}

func (s *kimiOAuthClientStub) StartDeviceFlow(_ context.Context, proxyURL string, headers kimi.DeviceHeaders) (*kimi.DeviceCodeResponse, error) {
	s.headers = append(s.headers, headers)
	s.proxies = append(s.proxies, proxyURL)
	return s.device, nil
}
func (s *kimiOAuthClientStub) PollDeviceToken(_ context.Context, _ string, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error) {
	s.headers = append(s.headers, headers)
	s.proxies = append(s.proxies, proxyURL)
	if len(s.polls) == 0 {
		return nil, errors.New("unexpected poll")
	}
	result := s.polls[0]
	s.polls = s.polls[1:]
	return result, nil
}
func (s *kimiOAuthClientStub) RefreshToken(_ context.Context, _ string, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error) {
	s.headers = append(s.headers, headers)
	s.proxies = append(s.proxies, proxyURL)
	return s.refresh, nil
}

type kimiDeviceSessionStoreStub struct {
	sessions   map[string]*KimiDeviceSession
	leaseTTLs  []time.Duration
	consumeErr error
}

type kimiProxyRepoStub struct {
	ProxyRepository
	proxy *Proxy
}

func (s *kimiProxyRepoStub) GetByID(_ context.Context, id int64) (*Proxy, error) {
	if s.proxy != nil && s.proxy.ID == id {
		return s.proxy, nil
	}
	return nil, ErrProxyNotFound
}

func (s *kimiDeviceSessionStoreStub) Create(_ context.Context, session *KimiDeviceSession, _ time.Duration) error {
	copy := *session
	s.sessions[session.ID] = &copy
	return nil
}
func (s *kimiDeviceSessionStoreStub) Get(_ context.Context, id string) (*KimiDeviceSession, error) {
	if value := s.sessions[id]; value != nil {
		copy := *value
		return &copy, nil
	}
	return nil, nil
}
func (s *kimiDeviceSessionStoreStub) AcquirePollLease(_ context.Context, id string, now time.Time, leaseTTL time.Duration) (*KimiDevicePollLease, error) {
	s.leaseTTLs = append(s.leaseTTLs, leaseTTL)
	value := s.sessions[id]
	if value == nil {
		return nil, nil
	}
	copy := *value
	return &KimiDevicePollLease{Session: &copy, Held: !now.Before(copy.NextPollAt)}, nil
}
func (s *kimiDeviceSessionStoreStub) CommitPoll(_ context.Context, _ *KimiDevicePollLease, updated *KimiDeviceSession) (bool, error) {
	copy := *updated
	s.sessions[updated.ID] = &copy
	return true, nil
}
func (s *kimiDeviceSessionStoreStub) ConsumeAuthorized(_ context.Context, id string) (*KimiDeviceSession, error) {
	if s.consumeErr != nil {
		return nil, s.consumeErr
	}
	value := s.sessions[id]
	if value == nil || value.Status != "authorized" {
		return nil, nil
	}
	delete(s.sessions, id)
	return value, nil
}
func (s *kimiDeviceSessionStoreStub) Cancel(_ context.Context, id string, _ time.Duration) (bool, error) {
	value := s.sessions[id]
	if value == nil {
		return false, nil
	}
	value.Status = ProviderOAuthSessionCancelled
	return true, nil
}
func (s *kimiDeviceSessionStoreStub) Delete(_ context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

func TestKimiDeviceAuthorizationSlowDownAndConsume(t *testing.T) {
	now := time.Date(2026, 7, 29, 5, 0, 0, 0, time.UTC)
	client := &kimiOAuthClientStub{
		device: &kimi.DeviceCodeResponse{DeviceCode: "device-secret", UserCode: "ABCD-EFGH", VerificationURI: "https://kimi.com/device", ExpiresIn: 600, Interval: 1},
		polls:  []*kimi.TokenResponse{{Error: "slow_down"}, {AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}},
	}
	store := &kimiDeviceSessionStoreStub{sessions: map[string]*KimiDeviceSession{}}
	service := NewKimiOAuthService(nil, client, store)
	service.now = func() time.Time { return now }

	started, err := service.StartDeviceAuthorization(context.Background(), nil)
	require.NoError(t, err)
	require.EqualValues(t, 5, started.Interval)
	require.NotEmpty(t, started.SessionID)

	now = now.Add(5 * time.Second)
	status, err := service.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "pending", status.Status)
	require.EqualValues(t, 10, status.Interval)
	require.Equal(t, []time.Duration{90 * time.Second}, store.leaseTTLs)

	now = now.Add(10 * time.Second)
	status, err = service.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "authorized", status.Status)
	require.Len(t, client.headers, 3)
	require.Equal(t, client.headers[0].DeviceID, client.headers[1].DeviceID)
	require.Equal(t, client.headers[0].DeviceID, client.headers[2].DeviceID)
	_, err = uuid.Parse(client.headers[0].DeviceID)
	require.NoError(t, err, "Kimi device ID should use UUID format")

	token, err := service.ConsumeAuthorizedSession(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "refresh", token.RefreshToken)
	_, err = service.ConsumeAuthorizedSession(context.Background(), started.SessionID)
	require.Error(t, err)
}

func TestKimiRefreshPreservesDeviceAndRotatedToken(t *testing.T) {
	client := &kimiOAuthClientStub{refresh: &kimi.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 1200}}
	service := NewKimiOAuthService(nil, client, &kimiDeviceSessionStoreStub{sessions: map[string]*KimiDeviceSession{}})
	account := &Account{Platform: PlatformKimi, Type: AccountTypeOAuth, Credentials: map[string]any{
		"refresh_token": "old-refresh", "device_id": "device-1", "device_name": "host-1", "device_model": "darwin/arm64",
	}}
	token, err := service.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-refresh", token.RefreshToken)
	require.Equal(t, "device-1", client.headers[0].DeviceID)
	credentials := service.BuildAccountCredentials(token)
	require.Equal(t, "https://api.kimi.com/coding", credentials["base_url"])
}

func TestKimiRefreshGeneratesDeviceIDForLegacyCredentials(t *testing.T) {
	client := &kimiOAuthClientStub{refresh: &kimi.TokenResponse{AccessToken: "new-access", ExpiresIn: 1200}}
	service := NewKimiOAuthService(nil, client, &kimiDeviceSessionStoreStub{sessions: map[string]*KimiDeviceSession{}})
	account := &Account{Platform: PlatformKimi, Type: AccountTypeOAuth, Credentials: map[string]any{
		"refresh_token": "legacy-refresh",
	}}

	token, err := service.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, client.headers)
	require.NotEmpty(t, client.headers[0].DeviceID)
	_, err = uuid.Parse(client.headers[0].DeviceID)
	require.NoError(t, err, "legacy Kimi credentials should receive a UUID device ID")
	require.Equal(t, client.headers[0].DeviceID, token.DeviceID)
	require.Equal(t, client.headers[0].DeviceID, service.BuildAccountCredentials(token)["device_id"])
}

func TestKimiDeviceAuthorizationPreservesProxyWithoutExposingTokens(t *testing.T) {
	proxyID := int64(17)
	client := &kimiOAuthClientStub{
		device: &kimi.DeviceCodeResponse{DeviceCode: "device-secret", UserCode: "SAFE-CODE", VerificationURI: "https://kimi.com/device", ExpiresIn: 600, Interval: 5},
		polls:  []*kimi.TokenResponse{{AccessToken: "access-secret", RefreshToken: "refresh-secret", ExpiresIn: 3600}},
	}
	proxyRepo := &kimiProxyRepoStub{proxy: &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 8080}}
	store := &kimiDeviceSessionStoreStub{sessions: map[string]*KimiDeviceSession{}}
	svc := NewKimiOAuthService(proxyRepo, client, store)
	now := time.Date(2026, 7, 29, 5, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	started, err := svc.StartDeviceAuthorization(context.Background(), &proxyID)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080", client.proxies[0])
	encoded, err := json.Marshal(started)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "access-secret")
	require.NotContains(t, string(encoded), "refresh-secret")

	now = now.Add(5 * time.Second)
	status, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "authorized", status.Status)
	require.Equal(t, "http://127.0.0.1:8080", client.proxies[1])
	encoded, err = json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "access-secret")
	require.NotContains(t, string(encoded), "refresh-secret")

	token, err := svc.ConsumeAuthorizedSession(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, &proxyID, token.ProxyID)
}

func TestKimiConsumeAuthorizedPreservesStoreFailure(t *testing.T) {
	service := NewKimiOAuthService(nil, &kimiOAuthClientStub{}, &kimiDeviceSessionStoreStub{
		sessions:   map[string]*KimiDeviceSession{},
		consumeErr: errors.New("redis unavailable"),
	})

	_, err := service.ConsumeAuthorizedSession(context.Background(), "session-1")
	require.Error(t, err)
	require.Equal(t, 503, infraerrors.Code(err))
	require.Equal(t, "KIMI_OAUTH_SESSION_STORE_FAILED", infraerrors.Reason(err))
	require.NotContains(t, infraerrors.Message(err), "access_token")
}

func TestKimiRefreshRejectsEmptyTokenResponse(t *testing.T) {
	service := NewKimiOAuthService(nil, &kimiOAuthClientStub{refresh: nil}, &kimiDeviceSessionStoreStub{sessions: map[string]*KimiDeviceSession{}})
	account := &Account{Platform: PlatformKimi, Type: AccountTypeOAuth, Credentials: map[string]any{
		"refresh_token": "refresh", "device_id": "device-1",
	}}

	_, err := service.RefreshAccountToken(context.Background(), account)
	require.Error(t, err)
	require.Equal(t, 502, infraerrors.Code(err))
	require.Equal(t, "KIMI_OAUTH_INVALID_TOKEN_RESPONSE", infraerrors.Reason(err))
}
