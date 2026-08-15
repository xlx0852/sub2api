package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type usageSettlementUpstreamFake struct {
	status int
}

func (f *usageSettlementUpstreamFake) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return &http.Response{StatusCode: f.status}, nil
}
func (f *usageSettlementUpstreamFake) DoWithTLS(*http.Request, string, int64, int, *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return &http.Response{StatusCode: f.status}, nil
}

type usageSettlementCoordinatorFake struct {
	enabled       bool
	authoritative bool
	startErr      error
	readyErr      error
	started       *domain.UsageSettlementAttempt
	payload       *domain.UsageSettlementPayloadV1
	fingerprint   string
	wakeCount     int
	providerID    string
	selectedGroup *int64
}

func (f *usageSettlementCoordinatorFake) Wake() { f.wakeCount++ }

func (f *usageSettlementCoordinatorFake) Enabled() bool       { return f.enabled }
func (f *usageSettlementCoordinatorFake) Authoritative() bool { return f.authoritative }
func (f *usageSettlementCoordinatorFake) EnabledFor(groupID *int64) bool {
	return f.enabled && usageSettlementTestGroupMatches(f.selectedGroup, groupID)
}
func (f *usageSettlementCoordinatorFake) AuthoritativeFor(groupID *int64) bool {
	return f.authoritative && usageSettlementTestGroupMatches(f.selectedGroup, groupID)
}
func usageSettlementTestGroupMatches(selected, actual *int64) bool {
	if selected == nil {
		return true
	}
	return actual != nil && *selected == *actual
}
func (f *usageSettlementCoordinatorFake) StartAttempt(_ context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error) {
	f.started = attempt
	return attempt, true, f.startErr
}
func (f *usageSettlementCoordinatorFake) ObserveProviderRequestID(_ context.Context, _ string, _ int64, _ string, providerRequestID string) (*domain.UsageSettlementAttempt, error) {
	f.providerID = providerRequestID
	return f.started, nil
}
func (f *usageSettlementCoordinatorFake) HandoffReady(_ context.Context, _ string, _ int64, fingerprint string, payload *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error) {
	f.fingerprint = fingerprint
	f.payload = payload
	return f.started, f.readyErr
}

func TestHandoffUsageSettlementPreservesCalculatedPayloadAndMode(t *testing.T) {
	for _, authoritative := range []bool{false, true} {
		t.Run(map[bool]string{false: "shadow", true: "authoritative"}[authoritative], func(t *testing.T) {
			coordinator := &usageSettlementCoordinatorFake{enabled: true, authoritative: authoritative}
			groupID := int64(31)
			log := &UsageLog{
				UserID: 11, APIKeyID: 12, AccountID: 13, RequestID: "local:req-1",
				Model: "claude-test", GroupID: &groupID, InputTokens: 10, OutputTokens: 5,
				ActualCost: 0.0000001, TotalCost: 0.25, BillingType: BillingTypeBalance,
			}
			params := &postUsageBillingParams{
				Cost: &CostBreakdown{ActualCost: 0.0000001, TotalCost: 0.25},
				User: &User{ID: 11}, APIKey: &APIKey{ID: 12},
				Account:            &Account{ID: 13, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
				RequestPayloadHash: "body-hash", AccountRateMultiplier: 1.5,
			}

			gotAuthoritative, err := handoffUsageSettlement(context.Background(), coordinator, log, params, PlatformAnthropic)
			require.NoError(t, err)
			require.Equal(t, authoritative, gotAuthoritative)
			require.NotNil(t, coordinator.started)
			require.Equal(t, "ab23c275c86f2ba91a9e9b778453f1903291005855b41c35facfaddef68fcad7", coordinator.started.RequestFingerprint)
			require.Len(t, coordinator.started.RequestFingerprint, 64)
			require.Equal(t, coordinator.started.RequestFingerprint, coordinator.fingerprint)
			require.NotNil(t, coordinator.payload)
			require.Equal(t, "local:req-1", coordinator.payload.Billing.RequestID)
			require.NotEqual(t, coordinator.fingerprint, coordinator.payload.Billing.RequestFingerprint)
			require.Equal(t, 0.000001, coordinator.payload.Billing.BalanceCost)
			require.Equal(t, 0.000001, coordinator.payload.UsageLog.ActualCost)
			if authoritative {
				require.Equal(t, 1, coordinator.wakeCount)
			} else {
				require.Zero(t, coordinator.wakeCount)
			}
		})
	}
}

func TestHandoffUsageSettlementLeavesUnselectedGroupOnLegacyPath(t *testing.T) {
	selected, other := int64(71), int64(72)
	coordinator := &usageSettlementCoordinatorFake{enabled: true, authoritative: true, selectedGroup: &selected}
	log := &UsageLog{
		UserID: 11, APIKeyID: 12, AccountID: 13, RequestID: "local:unselected",
		Model: "gpt-test", GroupID: &other,
	}
	params := &postUsageBillingParams{
		Cost: &CostBreakdown{}, User: &User{ID: 11}, APIKey: &APIKey{ID: 12, GroupID: &other},
		Account: &Account{ID: 13, Platform: PlatformOpenAI},
	}
	authoritative, err := handoffUsageSettlement(context.Background(), coordinator, log, params, PlatformOpenAI)
	require.NoError(t, err)
	require.False(t, authoritative)
	require.Nil(t, coordinator.started)
	require.Nil(t, coordinator.payload)
}

func TestUsageSettlementGroupContextCopiesValue(t *testing.T) {
	groupID := int64(81)
	ctx := WithUsageSettlementGroupID(context.Background(), &groupID)
	groupID = 82
	stored := UsageSettlementGroupIDFromContext(ctx)
	require.NotNil(t, stored)
	require.Equal(t, int64(81), *stored)
	require.Nil(t, UsageSettlementGroupIDFromContext(context.Background()))
}

func TestHandoffUsageSettlementFailsBeforeReadyWhenStartedPersistenceFails(t *testing.T) {
	coordinator := &usageSettlementCoordinatorFake{enabled: true, startErr: errors.New("insert failed")}
	log := &UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "local:req", Model: "m"}
	params := &postUsageBillingParams{
		Cost: &CostBreakdown{}, User: &User{ID: 1}, APIKey: &APIKey{ID: 2},
		Account: &Account{ID: 3, Platform: PlatformOpenAI},
	}
	_, err := handoffUsageSettlement(context.Background(), coordinator, log, params, PlatformOpenAI)
	require.EqualError(t, err, "insert failed")
	require.Nil(t, coordinator.payload)
}

func TestStartUsageSettlementAttemptUsesLocalIdentityAndRequestFingerprint(t *testing.T) {
	coordinator := &usageSettlementCoordinatorFake{enabled: true}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "request-42")
	err := startUsageSettlementAttempt(ctx, coordinator, UsageSettlementStartInput{
		RequestPayloadHash: "body-42",
		APIKey:             &APIKey{ID: 2},
		User:               &User{ID: 1},
		Account:            &Account{ID: 3, Platform: PlatformAnthropic},
		RequestType:        RequestTypeStream,
		InboundEndpoint:    "/v1/messages",
		UpstreamEndpoint:   "/v1/messages?beta=true",
	})
	require.NoError(t, err)
	require.NotNil(t, coordinator.started)
	require.Equal(t, "local:request-42", coordinator.started.BillingIdentity)
	require.Equal(t, "f985e092295c251535b6329396c13ad3b26354e3094a341f18cd204844de4d52", coordinator.started.RequestFingerprint)
	require.Len(t, coordinator.started.RequestFingerprint, 64)
	require.Equal(t, RequestTypeStream, coordinator.started.RequestType)
	require.NotNil(t, coordinator.started.ReconciliationDeadline)
	require.WithinDuration(t, time.Now().UTC().Add(defaultUsageSettlementReconciliationWindow), *coordinator.started.ReconciliationDeadline, time.Second)
}

func TestStartUsageSettlementAttemptPreservesExplicitReconciliationDeadline(t *testing.T) {
	coordinator := &usageSettlementCoordinatorFake{enabled: true}
	deadline := time.Now().UTC().Add(5 * time.Minute)
	err := startUsageSettlementAttempt(context.Background(), coordinator, UsageSettlementStartInput{
		BillingIdentity: "local:explicit-deadline", ReconciliationUntil: &deadline,
		APIKey: &APIKey{ID: 2}, User: &User{ID: 1},
		Account: &Account{ID: 3, Platform: PlatformOpenAI},
	})
	require.NoError(t, err)
	require.NotNil(t, coordinator.started)
	require.Equal(t, deadline, *coordinator.started.ReconciliationDeadline)
}

func TestUsageSettlementHTTPUpstreamStartsOnceOnlyAfterAcceptedResponse(t *testing.T) {
	coordinator := &usageSettlementCoordinatorFake{enabled: true}
	service := &OpenAIGatewayService{usageSettlement: coordinator}
	input := UsageSettlementStartInput{
		APIKey: &APIKey{ID: 2}, User: &User{ID: 1},
		Account:            &Account{ID: 3, Platform: PlatformOpenAI},
		RequestPayloadHash: "body",
	}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "accepted-1")
	ctx = service.WithUsageSettlementAttempt(ctx, input)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid/v1/responses", nil)
	require.NoError(t, err)

	rejected := withUsageSettlementHTTPUpstream(&usageSettlementUpstreamFake{status: http.StatusTooManyRequests})
	_, err = rejected.Do(request, "", 3, 1)
	require.NoError(t, err)
	require.Nil(t, coordinator.started)

	accepted := withUsageSettlementHTTPUpstream(&usageSettlementUpstreamFake{status: http.StatusOK})
	_, err = accepted.Do(request, "", 3, 1)
	require.NoError(t, err)
	_, err = accepted.DoWithTLS(request, "", 3, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, coordinator.started)
	require.Equal(t, "local:accepted-1", coordinator.started.BillingIdentity)
	notifyUsageSettlementProviderRequestID(ctx, "resp_stored_1")
	require.Equal(t, "resp_stored_1", coordinator.providerID)
}

func TestOpenAIResponseStoredRequiresExplicitTrue(t *testing.T) {
	require.True(t, openAIResponseStored([]byte(`{"type":"response.created","response":{"id":"resp_1","store":true}}`)))
	require.True(t, openAIResponseStored([]byte(`{"id":"resp_1","store":true}`)))
	require.False(t, openAIResponseStored([]byte(`{"response":{"id":"resp_1","store":false}}`)))
	require.False(t, openAIResponseStored([]byte(`{"response":{"id":"resp_1"}}`)))
}
