//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokOAuthClientStub struct {
	refreshResponse *xai.TokenResponse
	deviceResponse  *xai.DeviceCodeResponse
	pollResponses   []*xai.DeviceTokenResponse
	exchangeCalls   int
	pollCalls       int
}

func (s *grokOAuthClientStub) ExchangeCode(context.Context, string, string, string, string, string) (*xai.TokenResponse, error) {
	s.exchangeCalls++
	return &xai.TokenResponse{}, nil
}

func (s *grokOAuthClientStub) RefreshToken(context.Context, string, string, string) (*xai.TokenResponse, error) {
	return s.refreshResponse, nil
}

func (s *grokOAuthClientStub) RefreshTokenAtEndpoint(context.Context, string, string, string, string) (*xai.TokenResponse, error) {
	return s.refreshResponse, nil
}

func (s *grokOAuthClientStub) StartDeviceFlow(context.Context, string, string, string) (*xai.DeviceCodeResponse, error) {
	return s.deviceResponse, nil
}

func (s *grokOAuthClientStub) PollDeviceToken(context.Context, string, string, string, string) (*xai.DeviceTokenResponse, error) {
	if s.pollCalls >= len(s.pollResponses) {
		return nil, errors.New("unexpected device poll")
	}
	response := s.pollResponses[s.pollCalls]
	s.pollCalls++
	return response, nil
}

type providerOAuthSessionStoreStub struct {
	mu      sync.Mutex
	session *ProviderOAuthSession
}

func (s *providerOAuthSessionStoreStub) Create(_ context.Context, session *ProviderOAuthSession, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *session
	s.session = &copy
	return nil
}

func (s *providerOAuthSessionStoreStub) Get(_ context.Context, _ string) (*ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil, nil
	}
	copy := *s.session
	return &copy, nil
}

func (s *providerOAuthSessionStoreStub) AcquirePollLease(_ context.Context, _ string, now time.Time, _ time.Duration) (*ProviderOAuthSessionLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return &ProviderOAuthSessionLease{}, nil
	}
	copy := *s.session
	if copy.Status != ProviderOAuthSessionPending || copy.NextPollAtUnixMilli > now.UnixMilli() {
		return &ProviderOAuthSessionLease{Session: &copy}, nil
	}
	copy.Version++
	copy.PollLeaseID = "lease"
	s.session = &copy
	leaseCopy := copy
	return &ProviderOAuthSessionLease{Session: &leaseCopy, ID: "lease", Held: true}, nil
}

func (s *providerOAuthSessionStoreStub) CommitPoll(_ context.Context, lease *ProviderOAuthSessionLease, updated *ProviderOAuthSession) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != ProviderOAuthSessionPending || lease == nil || !lease.Held || s.session.Version != lease.Session.Version {
		return false, nil
	}
	copy := *updated
	copy.Version = s.session.Version + 1
	copy.PollLeaseID = ""
	s.session = &copy
	return true, nil
}

func (s *providerOAuthSessionStoreStub) Cancel(_ context.Context, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return false, nil
	}
	s.session.Status = ProviderOAuthSessionCancelled
	s.session.Payload = nil
	return true, nil
}

func (s *providerOAuthSessionStoreStub) ConsumeAuthorized(_ context.Context, _ string) (*ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != ProviderOAuthSessionAuthorized {
		return nil, nil
	}
	result := s.session
	s.session = nil
	return result, nil
}

func (s *providerOAuthSessionStoreStub) Delete(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	return nil
}

func TestGrokOAuthServiceRefreshTokenPreservesOriginalRefreshTokenWhenNotRotated(t *testing.T) {
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		refreshResponse: &xai.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		},
	})
	defer svc.Stop()

	info, err := svc.RefreshToken(context.Background(), "original-refresh-token", "", "client-id")
	require.NoError(t, err)
	require.Equal(t, "new-access-token", info.AccessToken)
	require.Equal(t, "original-refresh-token", info.RefreshToken)
	require.Equal(t, "client-id", info.ClientID)
}

func TestGrokOAuthServiceExchangeCodeRequiresStateForCallbackURLAndConsumesSession(t *testing.T) {
	client := &grokOAuthClientStub{}
	svc := NewGrokOAuthService(nil, client)
	defer svc.Stop()

	auth, err := svc.GenerateAuthURL(context.Background(), nil, "")
	require.NoError(t, err)

	_, err = svc.ExchangeCode(context.Background(), &GrokExchangeCodeInput{
		SessionID: auth.SessionID,
		Code:      "http://127.0.0.1:56121/callback?code=code-without-state",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "GROK_OAUTH_STATE_REQUIRED")
	require.Zero(t, client.exchangeCalls)

	_, err = svc.ExchangeCode(context.Background(), &GrokExchangeCodeInput{
		SessionID: auth.SessionID,
		Code:      "code-with-state",
		State:     auth.State,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "GROK_OAUTH_SESSION_NOT_FOUND")
	require.Zero(t, client.exchangeCalls)
}

func TestGrokOAuthServiceDeviceFlowPendingSlowDownAndAuthorized(t *testing.T) {
	store := &providerOAuthSessionStoreStub{}
	client := &grokOAuthClientStub{
		deviceResponse: &xai.DeviceCodeResponse{
			DeviceCode: "device-code", UserCode: "USER-CODE",
			VerificationURI: "https://x.ai/device", ExpiresIn: 900, Interval: 5,
			TokenEndpoint: "https://auth.x.ai/oauth2/token",
		},
		pollResponses: []*xai.DeviceTokenResponse{
			{Error: "authorization_pending"},
			{Error: "slow_down"},
			{TokenResponse: xai.TokenResponse{AccessToken: "access", RefreshToken: "refresh", IDToken: "id", TokenType: "Bearer", ExpiresIn: 3600}},
		},
	}
	svc := NewGrokOAuthService(nil, client, store)
	defer svc.Stop()
	current := time.Now().UTC().Truncate(time.Millisecond)
	svc.now = func() time.Time { return current }

	started, err := svc.StartDeviceAuthorization(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionPending, started.Status)
	require.Equal(t, "USER-CODE", started.UserCode)
	require.Equal(t, int64(5), started.Interval)

	pending, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionPending, pending.Status)
	require.Equal(t, 1, client.pollCalls)

	current = current.Add(5 * time.Second)
	slowed, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, int64(10), slowed.Interval)
	require.Equal(t, 2, client.pollCalls)

	current = current.Add(10 * time.Second)
	authorized, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionAuthorized, authorized.Status)
	require.Equal(t, 3, client.pollCalls)

	credential, err := svc.ConsumeDeviceAuthorization(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "access", credential.Token.AccessToken)
	require.Equal(t, "refresh", credential.Token.RefreshToken)
	require.Equal(t, "https://auth.x.ai/oauth2/token", credential.Token.TokenEndpoint)

	_, err = svc.ConsumeDeviceAuthorization(context.Background(), started.SessionID)
	require.Error(t, err)
}
