package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/testfailpoint"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/tidwall/gjson"
)

type usageSettlementAcceptedCallbackKey struct{}
type usageSettlementGroupIDContextKey struct{}

type usageSettlementAcceptedCallback struct {
	once                     sync.Once
	fn                       func() error
	observeProviderRequestID func(string) error
}

func WithUsageSettlementGroupID(ctx context.Context, groupID *int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if groupID == nil {
		return ctx
	}
	value := *groupID
	return context.WithValue(ctx, usageSettlementGroupIDContextKey{}, value)
}

func UsageSettlementGroupIDFromContext(ctx context.Context) *int64 {
	if ctx == nil {
		return nil
	}
	value, ok := ctx.Value(usageSettlementGroupIDContextKey{}).(int64)
	if !ok {
		return nil
	}
	return &value
}

func (s *OpenAIGatewayService) WithUsageSettlementAttempt(ctx context.Context, input UsageSettlementStartInput) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.usageSettlement == nil || input.APIKey == nil || !s.usageSettlement.EnabledFor(input.APIKey.GroupID) {
		return ctx
	}
	callback := &usageSettlementAcceptedCallback{fn: func() error {
		return s.StartUsageSettlementAttempt(ctx, input)
	}, observeProviderRequestID: func(providerRequestID string) error {
		return observeUsageSettlementProviderRequestID(ctx, s.usageSettlement, input, providerRequestID)
	}}
	return context.WithValue(ctx, usageSettlementAcceptedCallbackKey{}, callback)
}

func (s *GatewayService) WithUsageSettlementAttempt(ctx context.Context, input UsageSettlementStartInput) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.usageSettlement == nil || input.APIKey == nil || !s.usageSettlement.EnabledFor(input.APIKey.GroupID) {
		return ctx
	}
	callback := &usageSettlementAcceptedCallback{fn: func() error {
		return s.StartUsageSettlementAttempt(ctx, input)
	}, observeProviderRequestID: func(providerRequestID string) error {
		return observeUsageSettlementProviderRequestID(ctx, s.usageSettlement, input, providerRequestID)
	}}
	return context.WithValue(ctx, usageSettlementAcceptedCallbackKey{}, callback)
}

func notifyUsageSettlementProviderRequestID(ctx context.Context, providerRequestID string) {
	if ctx == nil || strings.TrimSpace(providerRequestID) == "" {
		return
	}
	callback, _ := ctx.Value(usageSettlementAcceptedCallbackKey{}).(*usageSettlementAcceptedCallback)
	if callback == nil || callback.observeProviderRequestID == nil {
		return
	}
	if err := callback.observeProviderRequestID(providerRequestID); err != nil {
		slog.Error("usage settlement provider request id persistence failed", "error", err)
	}
}

func observeUsageSettlementProviderRequestID(ctx context.Context, coordinator ports.UsageSettlementCoordinator, input UsageSettlementStartInput, providerRequestID string) error {
	if coordinator == nil || !coordinator.Enabled() {
		return nil
	}
	if input.APIKey == nil {
		return errors.New("usage settlement provider request id API key is missing")
	}
	identity := strings.TrimSpace(input.BillingIdentity)
	if identity == "" {
		identity = resolveUsageBillingRequestID(ctx, "")
	}
	requestFingerprint := usageSettlementRequestFingerprint(&UsageBillingCommand{
		RequestID: identity, RequestPayloadHash: input.RequestPayloadHash,
	})
	settlementCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	_, err := coordinator.ObserveProviderRequestID(settlementCtx, identity, input.APIKey.ID, requestFingerprint, providerRequestID)
	return err
}

func notifyUsageSettlementUpstreamAccepted(ctx context.Context) {
	if ctx == nil {
		return
	}
	callback, _ := ctx.Value(usageSettlementAcceptedCallbackKey{}).(*usageSettlementAcceptedCallback)
	if callback == nil || callback.fn == nil {
		return
	}
	callback.once.Do(func() {
		if err := callback.fn(); err != nil {
			slog.Error("usage settlement started persistence failed after upstream accepted", "error", err)
		}
	})
}

type usageSettlementHTTPUpstream struct {
	next HTTPUpstream
}

func withUsageSettlementHTTPUpstream(next HTTPUpstream) HTTPUpstream {
	if next == nil {
		return nil
	}
	if _, ok := next.(*usageSettlementHTTPUpstream); ok {
		return next
	}
	return &usageSettlementHTTPUpstream{next: next}
}

func (u *usageSettlementHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	response, err := u.next.Do(req, proxyURL, accountID, accountConcurrency)
	if err == nil && response != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		notifyUsageSettlementUpstreamAccepted(req.Context())
	}
	return response, err
}

func (u *usageSettlementHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	response, err := u.next.DoWithTLS(req, proxyURL, accountID, accountConcurrency, profile)
	if err == nil && response != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		notifyUsageSettlementUpstreamAccepted(req.Context())
	}
	return response, err
}

type UsageSettlementStartInput struct {
	BillingIdentity     string
	RequestPayloadHash  string
	APIKey              *APIKey
	User                *User
	Account             *Account
	RequestType         RequestType
	InboundEndpoint     string
	UpstreamEndpoint    string
	ProviderRequestID   string
	RecoveryContext     *domain.UsageSettlementRecoveryContextV1
	ReconciliationUntil *time.Time
}

const defaultUsageSettlementReconciliationWindow = 24 * time.Hour

func (s *GatewayService) StartUsageSettlementAttempt(ctx context.Context, input UsageSettlementStartInput) error {
	if s == nil {
		return nil
	}
	return startUsageSettlementAttempt(ctx, s.usageSettlement, input)
}

func (s *GatewayService) UsageSettlementEnabled() bool {
	return s != nil && s.usageSettlement != nil && s.usageSettlement.Enabled()
}

func (s *GatewayService) UsageSettlementEnabledFor(groupID *int64) bool {
	return s != nil && s.usageSettlement != nil && s.usageSettlement.EnabledFor(groupID)
}

func (s *OpenAIGatewayService) StartUsageSettlementAttempt(ctx context.Context, input UsageSettlementStartInput) error {
	if s == nil {
		return nil
	}
	return startUsageSettlementAttempt(ctx, s.usageSettlement, input)
}

func (s *OpenAIGatewayService) UsageSettlementEnabled() bool {
	return s != nil && s.usageSettlement != nil && s.usageSettlement.Enabled()
}

func (s *OpenAIGatewayService) UsageSettlementEnabledFor(groupID *int64) bool {
	return s != nil && s.usageSettlement != nil && s.usageSettlement.EnabledFor(groupID)
}

func startUsageSettlementAttempt(ctx context.Context, coordinator ports.UsageSettlementCoordinator, input UsageSettlementStartInput) error {
	if coordinator == nil || input.APIKey == nil || !coordinator.EnabledFor(input.APIKey.GroupID) {
		return nil
	}
	if input.APIKey == nil || input.User == nil || input.Account == nil {
		return errors.New("usage settlement start subjects are incomplete")
	}
	identity := strings.TrimSpace(input.BillingIdentity)
	if identity == "" {
		identity = resolveUsageBillingRequestID(ctx, "")
	}
	requestFingerprint := usageSettlementRequestFingerprint(&UsageBillingCommand{
		RequestID: identity, RequestPayloadHash: input.RequestPayloadHash,
	})
	startedAt := time.Now().UTC()
	reconciliationDeadline := input.ReconciliationUntil
	if reconciliationDeadline == nil {
		deadline := startedAt.Add(defaultUsageSettlementReconciliationWindow)
		reconciliationDeadline = &deadline
	}
	attempt := &domain.UsageSettlementAttempt{
		BillingIdentity:        identity,
		APIKeyID:               input.APIKey.ID,
		RequestFingerprint:     requestFingerprint,
		UserID:                 input.User.ID,
		AccountID:              input.Account.ID,
		GroupID:                input.APIKey.GroupID,
		Provider:               input.Account.Platform,
		RequestType:            input.RequestType,
		ProviderRequestID:      input.ProviderRequestID,
		RecoveryContext:        input.RecoveryContext,
		InboundEndpoint:        input.InboundEndpoint,
		UpstreamEndpoint:       input.UpstreamEndpoint,
		State:                  domain.UsageSettlementStateStarted,
		ReconciliationDeadline: reconciliationDeadline,
		StartedAt:              startedAt,
	}
	settlementCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	_, _, err := coordinator.StartAttempt(settlementCtx, attempt)
	return err
}

func NewOpenAIUsageSettlementRecoveryContext(
	body []byte,
	originalModel string,
	mappedModel string,
	channelFields ChannelUsageFields,
	userAgent string,
	ipAddress string,
	quotaPlatform string,
) *domain.UsageSettlementRecoveryContextV1 {
	store := gjson.GetBytes(body, "store")
	if store.Exists() && !store.Bool() {
		return nil
	}
	originalModel = strings.TrimSpace(originalModel)
	if originalModel == "" {
		return nil
	}
	billingModel := firstNonEmpty(strings.TrimSpace(mappedModel), originalModel)
	serviceTier := extractOpenAIServiceTierFromBody(body)
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, billingModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)
	context := &domain.UsageSettlementRecoveryContextV1{
		Version:            1,
		RequestPayloadHash: HashUsageRequestPayload(body),
		OriginalModel:      originalModel,
		MappedModel:        strings.TrimSpace(mappedModel),
		UserAgent:          strings.TrimSpace(userAgent),
		IPAddress:          strings.TrimSpace(ipAddress),
		QuotaPlatform:      strings.TrimSpace(quotaPlatform),
		ChannelUsageFields: channelFields,
	}
	if serviceTier != nil {
		context.ServiceTier = strings.TrimSpace(*serviceTier)
	}
	if reasoningEffort != nil {
		context.ReasoningEffort = strings.TrimSpace(*reasoningEffort)
	}
	if err := context.Validate(); err != nil {
		return nil
	}
	return context
}

func usageSettlementRequestFingerprint(command *UsageBillingCommand) string {
	if command == nil {
		return ""
	}
	var source string
	if payloadHash := strings.TrimSpace(command.RequestPayloadHash); payloadHash != "" {
		source = "payload:" + payloadHash
	} else if identity := strings.TrimSpace(command.RequestID); identity != "" {
		source = "identity:" + identity
	}
	if source == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func handoffUsageSettlement(
	ctx context.Context,
	coordinator ports.UsageSettlementCoordinator,
	usageLog *UsageLog,
	params *postUsageBillingParams,
	quotaPlatform string,
) (bool, error) {
	if coordinator == nil || usageLog == nil || !coordinator.EnabledFor(usageLog.GroupID) {
		return false, nil
	}
	if params == nil {
		return coordinator.AuthoritativeFor(usageLog.GroupID), nil
	}
	command := buildUsageBillingCommand(usageLog.RequestID, usageLog, params)
	applyCanonicalUsageBillingAmounts(usageLog, params, command)
	if command == nil {
		return coordinator.AuthoritativeFor(usageLog.GroupID), nil
	}
	requestFingerprint := usageSettlementRequestFingerprint(command)
	startedAt := usageLog.CreatedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	reconciliationDeadline := startedAt.Add(defaultUsageSettlementReconciliationWindow)
	attempt := &domain.UsageSettlementAttempt{
		BillingIdentity:        command.RequestID,
		APIKeyID:               command.APIKeyID,
		RequestFingerprint:     requestFingerprint,
		UserID:                 command.UserID,
		AccountID:              command.AccountID,
		GroupID:                usageLog.GroupID,
		Provider:               params.Account.Platform,
		RequestType:            usageLog.EffectiveRequestType(),
		InboundEndpoint:        usageSettlementStringValue(usageLog.InboundEndpoint),
		UpstreamEndpoint:       usageSettlementStringValue(usageLog.UpstreamEndpoint),
		State:                  domain.UsageSettlementStateStarted,
		ReconciliationDeadline: &reconciliationDeadline,
		StartedAt:              startedAt,
	}
	settlementCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	if _, _, err := coordinator.StartAttempt(settlementCtx, attempt); err != nil {
		return coordinator.AuthoritativeFor(usageLog.GroupID), err
	}
	payload := domain.NewUsageSettlementPayloadV1(command, usageLog, quotaPlatform)
	if _, err := coordinator.HandoffReady(settlementCtx, command.RequestID, command.APIKeyID, requestFingerprint, payload); err != nil {
		return coordinator.AuthoritativeFor(usageLog.GroupID), err
	}
	// The terminal_parsed crash seam intentionally fires only after the
	// synchronous durable handoff. A SIGKILL at this boundary must leave a ready
	// payload that a restarted instance can claim.
	testfailpoint.ReachSettlement(testfailpoint.SettlementTerminalParsed)
	testfailpoint.ReachSettlement(testfailpoint.SettlementEventPersisted)
	if coordinator.AuthoritativeFor(usageLog.GroupID) {
		coordinator.Wake()
	}
	return coordinator.AuthoritativeFor(usageLog.GroupID), nil
}

func usageSettlementStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
