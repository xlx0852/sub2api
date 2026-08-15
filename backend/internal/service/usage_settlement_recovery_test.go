package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type usageSettlementRecoveryAPIKeyRepo struct {
	APIKeyRepository
	key *APIKey
}

func (r *usageSettlementRecoveryAPIKeyRepo) GetByID(context.Context, int64) (*APIKey, error) {
	return r.key, nil
}

type usageSettlementRecoveryHTTPUpstream struct {
	status int
	body   string
	req    *http.Request
}

func (u *usageSettlementRecoveryHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.req = req
	return &http.Response{StatusCode: u.status, Body: io.NopCloser(strings.NewReader(u.body)), Header: make(http.Header)}, nil
}

func (u *usageSettlementRecoveryHTTPUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

func TestUsageSettlementRecoveryRetrievesStoredResponseAndHandsOffReady(t *testing.T) {
	user := &User{ID: 101}
	apiKey := &APIKey{ID: 102, UserID: user.ID, User: user}
	account := Account{ID: 103, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-only-key", "base_url": "https://api.openai.com/v1",
	}}
	upstream := &usageSettlementRecoveryHTTPUpstream{status: http.StatusOK, body: `{
		"id":"resp_stored_1","object":"response","status":"completed","store":true,
		"model":"gpt-5.1","usage":{"input_tokens":15,"output_tokens":4,"input_tokens_details":{"cached_tokens":5}}
	}`}
	coordinator := &usageSettlementCoordinatorFake{enabled: true, authoritative: true}
	svc := newOpenAIRecordUsageServiceForTest(nil, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.accountRepo = stubOpenAIAccountRepo{accounts: []Account{account}}
	svc.httpUpstream = upstream
	svc.SetUsageSettlementCoordinator(coordinator)
	recovery := NewUsageSettlementRecovery(svc, svc.accountRepo, &usageSettlementRecoveryAPIKeyRepo{key: apiKey})
	requestHash := HashUsageRequestPayload([]byte(`{"model":"gpt-5.1","stream":true}`))
	attempt := &domain.UsageSettlementAttempt{
		BillingIdentity: "client:recover-1", APIKeyID: apiKey.ID, UserID: user.ID, AccountID: account.ID,
		Provider: PlatformOpenAI, ProviderRequestID: "resp_stored_1", RequestType: RequestTypeStream,
		InboundEndpoint: "/v1/responses", UpstreamEndpoint: "/v1/responses",
		RequestFingerprint: usageSettlementRequestFingerprint(&UsageBillingCommand{RequestID: "client:recover-1", RequestPayloadHash: requestHash}),
		RecoveryContext: &domain.UsageSettlementRecoveryContextV1{
			Version: 1, RequestPayloadHash: requestHash, OriginalModel: "gpt-5.1", MappedModel: "gpt-5.1",
			UserAgent: "recovery-test", IPAddress: "192.0.2.1", QuotaPlatform: PlatformOpenAI,
		},
	}

	recovered, err := recovery.RecoverUsageSettlement(context.Background(), attempt)
	require.NoError(t, err)
	require.True(t, recovered)
	require.NotNil(t, coordinator.payload)
	require.Equal(t, "client:recover-1", coordinator.payload.Billing.RequestID)
	require.Equal(t, 10, coordinator.payload.UsageLog.InputTokens)
	require.Equal(t, 5, coordinator.payload.UsageLog.CacheReadTokens)
	require.Equal(t, 4, coordinator.payload.UsageLog.OutputTokens)
	require.NotNil(t, coordinator.payload.UsageLog.UserAgent)
	require.Equal(t, "recovery-test", *coordinator.payload.UsageLog.UserAgent)
	require.NotNil(t, upstream.req)
	require.Equal(t, http.MethodGet, upstream.req.Method)
	require.Equal(t, "/v1/responses/resp_stored_1", upstream.req.URL.Path)
	require.Equal(t, "Bearer test-only-key", upstream.req.Header.Get("Authorization"))
}

func TestUsageSettlementRecoveryLeavesRetrieve404ForReconciliation(t *testing.T) {
	user := &User{ID: 201}
	apiKey := &APIKey{ID: 202, UserID: user.ID, User: user}
	account := Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-only-key", "base_url": "https://api.openai.com/v1",
	}}
	upstream := &usageSettlementRecoveryHTTPUpstream{status: http.StatusNotFound, body: `{}`}
	svc := newOpenAIRecordUsageServiceForTest(nil, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.accountRepo = stubOpenAIAccountRepo{accounts: []Account{account}}
	svc.httpUpstream = upstream
	recovery := NewUsageSettlementRecovery(svc, svc.accountRepo, &usageSettlementRecoveryAPIKeyRepo{key: apiKey})
	attempt := &domain.UsageSettlementAttempt{
		APIKeyID: apiKey.ID, UserID: user.ID, AccountID: account.ID, Provider: PlatformOpenAI,
		ProviderRequestID: "resp_missing", RecoveryContext: &domain.UsageSettlementRecoveryContextV1{Version: 1},
	}
	recovered, err := recovery.RecoverUsageSettlement(context.Background(), attempt)
	require.NoError(t, err)
	require.False(t, recovered)
}
