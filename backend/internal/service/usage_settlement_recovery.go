package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/tidwall/gjson"
)

const maxUsageSettlementRetrieveBodyBytes = 2 << 20

type UsageSettlementRecovery struct {
	openAI   *OpenAIGatewayService
	accounts ports.AccountRepository
	apiKeys  ports.APIKeyRepository
}

func NewUsageSettlementRecovery(openAI *OpenAIGatewayService, accounts ports.AccountRepository, apiKeys ports.APIKeyRepository) *UsageSettlementRecovery {
	return &UsageSettlementRecovery{openAI: openAI, accounts: accounts, apiKeys: apiKeys}
}

func (r *UsageSettlementRecovery) RecoverUsageSettlement(ctx context.Context, attempt *domain.UsageSettlementAttempt) (bool, error) {
	if r == nil || r.openAI == nil || r.accounts == nil || r.apiKeys == nil || attempt == nil || attempt.RecoveryContext == nil {
		return false, nil
	}
	providerRequestID := strings.TrimSpace(attempt.ProviderRequestID)
	if providerRequestID == "" {
		return false, nil
	}
	if attempt.Provider != PlatformOpenAI && attempt.Provider != PlatformGrok {
		return false, nil
	}
	account, err := r.accounts.GetByID(ctx, attempt.AccountID)
	if err != nil {
		return false, err
	}
	if account == nil || account.Type != AccountTypeAPIKey || account.Platform != attempt.Provider {
		return false, nil
	}
	apiKey, err := r.apiKeys.GetByID(ctx, attempt.APIKeyID)
	if err != nil {
		return false, err
	}
	if apiKey == nil || apiKey.User == nil || apiKey.User.ID != attempt.UserID {
		return false, errors.New("usage settlement recovery subject mismatch")
	}
	body, err := r.retrieveStoredResponse(ctx, account, providerRequestID)
	if err != nil || len(body) == 0 {
		return false, err
	}
	usage, ok := extractOpenAIUsageFromJSONBytes(body)
	if !ok {
		return false, nil
	}
	responseID := extractOpenAIResponseIDFromJSONBytes(body)
	if responseID != providerRequestID {
		return false, domain.ErrUsageSettlementProviderIDConflict
	}
	recovery := attempt.RecoveryContext
	result := &OpenAIForwardResult{
		RequestID:     attempt.BillingIdentity,
		ResponseID:    responseID,
		ResponseModel: extractOpenAIResponseModelFromJSONBytes(body),
		Usage:         usage,
		Model:         recovery.OriginalModel,
		BillingModel:  recovery.MappedModel,
		UpstreamModel: extractOpenAIResponseModelFromJSONBytes(body),
		Stream:        attempt.RequestType == RequestTypeStream,
	}
	if recovery.ServiceTier != "" {
		value := recovery.ServiceTier
		result.ServiceTier = &value
	} else if value := strings.TrimSpace(gjson.GetBytes(body, "service_tier").String()); value != "" {
		result.ServiceTier = &value
	}
	if recovery.ReasoningEffort != "" {
		value := recovery.ReasoningEffort
		result.ReasoningEffort = &value
	}
	err = r.openAI.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result:             result,
		APIKey:             apiKey,
		User:               apiKey.User,
		Account:            account,
		InboundEndpoint:    attempt.InboundEndpoint,
		UpstreamEndpoint:   attempt.UpstreamEndpoint,
		UserAgent:          recovery.UserAgent,
		IPAddress:          recovery.IPAddress,
		RequestPayloadHash: recovery.RequestPayloadHash,
		QuotaPlatform:      recovery.QuotaPlatform,
		ChannelUsageFields: recovery.ChannelUsageFields,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *UsageSettlementRecovery) retrieveStoredResponse(ctx context.Context, account *Account, responseID string) ([]byte, error) {
	baseURL := ""
	switch account.Platform {
	case PlatformOpenAI:
		baseURL = account.GetOpenAIBaseURL()
	case PlatformGrok:
		if !account.GrokUsesOfficialAPI() {
			return nil, nil
		}
		baseURL = account.GetGrokOfficialBaseURL()
	default:
		return nil, nil
	}
	validated, err := r.openAI.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	target := strings.TrimRight(buildOpenAIEndpointURL(validated, "/v1/responses"), "/") + "/" + url.PathEscape(responseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	token, _, err := r.openAI.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	headers, err := r.openAI.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := r.openAI.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("stored response retrieve returned nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stored response retrieve returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUsageSettlementRetrieveBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxUsageSettlementRetrieveBodyBytes {
		return nil, errors.New("stored response retrieve body exceeds limit")
	}
	return body, nil
}
