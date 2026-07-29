//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type openAIDeviceOAuthClientStub struct {
	pollResponses []*openai.DeviceAuthorizationResponse
	pollCalls     int
	exchangeCalls int
}

func (s *openAIDeviceOAuthClientStub) RequestDeviceCode(context.Context, string, string) (*openai.DeviceCodeResponse, error) {
	return &openai.DeviceCodeResponse{
		DeviceAuthID: "device-auth",
		UserCode:     "ABCD-EFGH",
		Interval:     json.RawMessage(`"5"`),
	}, nil
}

func (s *openAIDeviceOAuthClientStub) PollDeviceAuthorization(context.Context, string, string, string) (*openai.DeviceAuthorizationResponse, error) {
	if s.pollCalls >= len(s.pollResponses) {
		return nil, errors.New("unexpected device poll")
	}
	response := s.pollResponses[s.pollCalls]
	s.pollCalls++
	return response, nil
}

func (s *openAIDeviceOAuthClientStub) ExchangeCode(_ context.Context, code, verifier, redirectURI, _ string, clientID string) (*openai.TokenResponse, error) {
	s.exchangeCalls++
	if code != "authorization-code" || verifier != "verifier" || redirectURI != openai.DeviceExchangeRedirect || clientID != openai.ClientID {
		return nil, errors.New("unexpected exchange input")
	}
	return &openai.TokenResponse{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}, nil
}

func (s *openAIDeviceOAuthClientStub) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("unexpected refresh")
}

func (s *openAIDeviceOAuthClientStub) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("unexpected refresh")
}

func TestOpenAIOAuthServiceDeviceFlowPendingAuthorizedAndConsumeOnce(t *testing.T) {
	store := &providerOAuthSessionStoreStub{}
	client := &openAIDeviceOAuthClientStub{pollResponses: []*openai.DeviceAuthorizationResponse{
		{Pending: true},
		{AuthorizationCode: "authorization-code", CodeVerifier: "verifier", CodeChallenge: "challenge"},
	}}
	svc := NewOpenAIOAuthService(nil, client, store)
	defer svc.Stop()
	current := time.Now().UTC().Truncate(time.Millisecond)
	svc.now = func() time.Time { return current }

	started, err := svc.StartDeviceAuthorization(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionPending, started.Status)
	require.Equal(t, "ABCD-EFGH", started.UserCode)
	require.Equal(t, int64(5), started.Interval)

	pending, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionPending, pending.Status)
	require.Equal(t, 1, client.pollCalls)

	current = current.Add(5 * time.Second)
	authorized, err := svc.GetDeviceAuthorizationStatus(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, ProviderOAuthSessionAuthorized, authorized.Status)
	require.Equal(t, 2, client.pollCalls)
	require.Equal(t, 1, client.exchangeCalls)

	credential, err := svc.ConsumeDeviceAuthorization(context.Background(), started.SessionID)
	require.NoError(t, err)
	require.Equal(t, "access", credential.Token.AccessToken)
	require.Equal(t, "refresh", credential.Token.RefreshToken)
	require.Equal(t, openai.ClientID, credential.Token.ClientID)

	_, err = svc.ConsumeDeviceAuthorization(context.Background(), started.SessionID)
	require.Error(t, err)
}

func TestParseOpenAIDevicePollInterval(t *testing.T) {
	require.Equal(t, 7*time.Second, parseOpenAIDevicePollInterval(json.RawMessage(`"7"`)))
	require.Equal(t, 9*time.Second, parseOpenAIDevicePollInterval(json.RawMessage(`9`)))
	require.Equal(t, openAIDeviceDefaultInterval, parseOpenAIDevicePollInterval(json.RawMessage(`"invalid"`)))
	require.Equal(t, openAIDeviceDefaultInterval, parseOpenAIDevicePollInterval(nil))
}
