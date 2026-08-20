package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const UsageSettlementPayloadVersionV1 = 1

type UsageSettlementState string

const (
	UsageSettlementStateStarted                UsageSettlementState = "started"
	UsageSettlementStateReady                  UsageSettlementState = "ready"
	UsageSettlementStateProcessing             UsageSettlementState = "processing"
	UsageSettlementStateSettled                UsageSettlementState = "settled"
	UsageSettlementStateReconciliationRequired UsageSettlementState = "reconciliation_required"
	UsageSettlementStateFailedRetryable        UsageSettlementState = "failed_retryable"
	UsageSettlementStateFailedTerminal         UsageSettlementState = "failed_terminal"
)

var (
	ErrUsageSettlementIdentityRequired    = errors.New("usage settlement billing identity is required")
	ErrUsageSettlementFingerprintRequired = errors.New("usage settlement request fingerprint is required")
	ErrUsageSettlementFingerprintConflict = errors.New("usage settlement request fingerprint conflict")
	ErrUsageSettlementPayloadRequired     = errors.New("usage settlement payload is required")
	ErrUsageSettlementPayloadConflict     = errors.New("usage settlement payload conflict")
	ErrUsageSettlementProviderIDConflict  = errors.New("usage settlement provider request id conflict")
	ErrUsageSettlementSensitivePayload    = errors.New("usage settlement payload contains a forbidden sensitive field")
	ErrUsageSettlementInvalidTransition   = errors.New("invalid usage settlement state transition")
	ErrUsageSettlementLeaseConflict       = errors.New("usage settlement lease conflict")
)

func (s UsageSettlementState) IsValid() bool {
	switch s {
	case UsageSettlementStateStarted,
		UsageSettlementStateReady,
		UsageSettlementStateProcessing,
		UsageSettlementStateSettled,
		UsageSettlementStateReconciliationRequired,
		UsageSettlementStateFailedRetryable,
		UsageSettlementStateFailedTerminal:
		return true
	default:
		return false
	}
}

func CanTransitionUsageSettlement(from, to UsageSettlementState) bool {
	if from == to {
		return true
	}
	switch from {
	case UsageSettlementStateStarted:
		return to == UsageSettlementStateReady || to == UsageSettlementStateReconciliationRequired || to == UsageSettlementStateFailedTerminal
	case UsageSettlementStateReady:
		return to == UsageSettlementStateProcessing || to == UsageSettlementStateFailedTerminal
	case UsageSettlementStateProcessing:
		return to == UsageSettlementStateSettled || to == UsageSettlementStateFailedRetryable || to == UsageSettlementStateFailedTerminal || to == UsageSettlementStateReconciliationRequired
	case UsageSettlementStateReconciliationRequired:
		return to == UsageSettlementStateReady || to == UsageSettlementStateFailedTerminal
	case UsageSettlementStateFailedRetryable:
		return to == UsageSettlementStateProcessing || to == UsageSettlementStateFailedTerminal
	default:
		return false
	}
}

func ValidateUsageSettlementTransition(from, to UsageSettlementState) error {
	if !from.IsValid() || !to.IsValid() || !CanTransitionUsageSettlement(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrUsageSettlementInvalidTransition, from, to)
	}
	return nil
}

type UsageSettlementAttempt struct {
	ID                     int64
	BillingIdentity        string
	APIKeyID               int64
	RequestFingerprint     string
	UserID                 int64
	AccountID              int64
	GroupID                *int64
	Provider               string
	RequestType            RequestType
	ProviderRequestID      string
	RecoveryContext        *UsageSettlementRecoveryContextV1
	InboundEndpoint        string
	UpstreamEndpoint       string
	Authoritative          bool
	State                  UsageSettlementState
	PayloadVersion         int
	Payload                *UsageSettlementPayloadV1
	PayloadHash            string
	LeaseOwner             string
	LeaseExpiresAt         *time.Time
	RetryCount             int
	NextAttemptAt          *time.Time
	ReconciliationDeadline *time.Time
	LastErrorCode          string
	LastErrorMessage       string
	StartedAt              time.Time
	ReadyAt                *time.Time
	SettledAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type UsageSettlementRecoveryContextV1 struct {
	Version            int                `json:"version"`
	RequestPayloadHash string             `json:"request_payload_hash"`
	OriginalModel      string             `json:"original_model"`
	MappedModel        string             `json:"mapped_model,omitempty"`
	ServiceTier        string             `json:"service_tier,omitempty"`
	ReasoningEffort    string             `json:"reasoning_effort,omitempty"`
	UserAgent          string             `json:"user_agent,omitempty"`
	IPAddress          string             `json:"ip_address,omitempty"`
	QuotaPlatform      string             `json:"quota_platform,omitempty"`
	ChannelUsageFields ChannelUsageFields `json:"channel_usage_fields"`
}

func (c *UsageSettlementRecoveryContextV1) Validate() error {
	if c == nil {
		return nil
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported usage settlement recovery context version: %d", c.Version)
	}
	c.RequestPayloadHash = strings.TrimSpace(c.RequestPayloadHash)
	c.OriginalModel = strings.TrimSpace(c.OriginalModel)
	c.MappedModel = strings.TrimSpace(c.MappedModel)
	c.ServiceTier = strings.TrimSpace(c.ServiceTier)
	c.ReasoningEffort = strings.TrimSpace(c.ReasoningEffort)
	c.QuotaPlatform = strings.TrimSpace(c.QuotaPlatform)
	if c.RequestPayloadHash == "" || c.OriginalModel == "" {
		return errors.New("usage settlement recovery context requires request hash and original model")
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return ValidateUsageSettlementPayloadJSON(encoded)
}

func (a *UsageSettlementAttempt) Normalize() {
	if a == nil {
		return
	}
	a.BillingIdentity = strings.TrimSpace(a.BillingIdentity)
	a.RequestFingerprint = strings.TrimSpace(a.RequestFingerprint)
	a.Provider = strings.ToLower(strings.TrimSpace(a.Provider))
	a.ProviderRequestID = strings.TrimSpace(a.ProviderRequestID)
	a.InboundEndpoint = strings.TrimSpace(a.InboundEndpoint)
	a.UpstreamEndpoint = strings.TrimSpace(a.UpstreamEndpoint)
	a.LeaseOwner = strings.TrimSpace(a.LeaseOwner)
	a.LastErrorCode = strings.TrimSpace(a.LastErrorCode)
	a.LastErrorMessage = strings.TrimSpace(a.LastErrorMessage)
	if a.State == "" {
		a.State = UsageSettlementStateStarted
	}
	a.RequestType = a.RequestType.Normalize()
}

func (a *UsageSettlementAttempt) ValidateStarted() error {
	if a == nil {
		return errors.New("usage settlement attempt is nil")
	}
	a.Normalize()
	if a.BillingIdentity == "" {
		return ErrUsageSettlementIdentityRequired
	}
	if a.RequestFingerprint == "" {
		return ErrUsageSettlementFingerprintRequired
	}
	if len(a.RequestFingerprint) > 64 {
		return errors.New("usage settlement request fingerprint exceeds 64 characters")
	}
	if a.APIKeyID <= 0 || a.UserID <= 0 || a.AccountID <= 0 {
		return errors.New("usage settlement user, api key, and account ids must be positive")
	}
	if a.Provider == "" {
		return errors.New("usage settlement provider is required")
	}
	if err := a.RecoveryContext.Validate(); err != nil {
		return err
	}
	if a.State != UsageSettlementStateStarted {
		return fmt.Errorf("%w: initial state %s", ErrUsageSettlementInvalidTransition, a.State)
	}
	return nil
}

// CanRebindStartedAccount reports whether a still-open started row may move to
// a later accepted account for the same identity, key, user, and fingerprint.
// Ready or later rows stay pinned; a payload means billing subjects are frozen.
func (a *UsageSettlementAttempt) CanRebindStartedAccount(userID, accountID int64, fingerprint string) bool {
	if a == nil || a.State != UsageSettlementStateStarted {
		return false
	}
	if a.Payload != nil || strings.TrimSpace(a.PayloadHash) != "" {
		return false
	}
	if a.RequestFingerprint != strings.TrimSpace(fingerprint) {
		return false
	}
	if a.UserID != userID || userID <= 0 || accountID <= 0 {
		return false
	}
	return a.AccountID != accountID
}

type UsageBillingSnapshot struct {
	RequestID           string  `json:"request_id"`
	APIKeyID            int64   `json:"api_key_id"`
	RequestFingerprint  string  `json:"request_fingerprint"`
	RequestPayloadHash  string  `json:"request_payload_hash,omitempty"`
	UserID              int64   `json:"user_id"`
	AccountID           int64   `json:"account_id"`
	SubscriptionID      *int64  `json:"subscription_id,omitempty"`
	AccountType         string  `json:"account_type"`
	Model               string  `json:"model"`
	ServiceTier         string  `json:"service_tier,omitempty"`
	ReasoningEffort     string  `json:"reasoning_effort,omitempty"`
	BillingType         int8    `json:"billing_type"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	ImageCount          int     `json:"image_count"`
	MediaType           string  `json:"media_type,omitempty"`
	BalanceCost         float64 `json:"balance_cost"`
	SubscriptionCost    float64 `json:"subscription_cost"`
	APIKeyQuotaCost     float64 `json:"api_key_quota_cost"`
	APIKeyRateLimitCost float64 `json:"api_key_rate_limit_cost"`
	AccountQuotaCost    float64 `json:"account_quota_cost"`
}

func NewUsageBillingSnapshot(command *UsageBillingCommand) UsageBillingSnapshot {
	if command == nil {
		return UsageBillingSnapshot{}
	}
	clone := *command
	clone.Normalize()
	return UsageBillingSnapshot{
		RequestID: clone.RequestID, APIKeyID: clone.APIKeyID, RequestFingerprint: clone.RequestFingerprint,
		RequestPayloadHash: clone.RequestPayloadHash, UserID: clone.UserID, AccountID: clone.AccountID,
		SubscriptionID: clone.SubscriptionID, AccountType: clone.AccountType, Model: clone.Model,
		ServiceTier: clone.ServiceTier, ReasoningEffort: clone.ReasoningEffort, BillingType: clone.BillingType,
		InputTokens: clone.InputTokens, OutputTokens: clone.OutputTokens, CacheCreationTokens: clone.CacheCreationTokens,
		CacheReadTokens: clone.CacheReadTokens, ImageCount: clone.ImageCount, MediaType: clone.MediaType,
		BalanceCost: clone.BalanceCost, SubscriptionCost: clone.SubscriptionCost, APIKeyQuotaCost: clone.APIKeyQuotaCost,
		APIKeyRateLimitCost: clone.APIKeyRateLimitCost, AccountQuotaCost: clone.AccountQuotaCost,
	}
}

func (s UsageBillingSnapshot) Command() *UsageBillingCommand {
	return &UsageBillingCommand{
		RequestID: s.RequestID, APIKeyID: s.APIKeyID, RequestFingerprint: s.RequestFingerprint,
		RequestPayloadHash: s.RequestPayloadHash, UserID: s.UserID, AccountID: s.AccountID,
		SubscriptionID: s.SubscriptionID, AccountType: s.AccountType, Model: s.Model,
		ServiceTier: s.ServiceTier, ReasoningEffort: s.ReasoningEffort, BillingType: s.BillingType,
		InputTokens: s.InputTokens, OutputTokens: s.OutputTokens, CacheCreationTokens: s.CacheCreationTokens,
		CacheReadTokens: s.CacheReadTokens, ImageCount: s.ImageCount, MediaType: s.MediaType,
		BalanceCost: s.BalanceCost, SubscriptionCost: s.SubscriptionCost, APIKeyQuotaCost: s.APIKeyQuotaCost,
		APIKeyRateLimitCost: s.APIKeyRateLimitCost, AccountQuotaCost: s.AccountQuotaCost,
	}
}

type UsageLogSnapshot struct {
	UserID                int64          `json:"user_id"`
	APIKeyID              int64          `json:"api_key_id"`
	AccountID             int64          `json:"account_id"`
	RequestID             string         `json:"request_id"`
	Model                 string         `json:"model"`
	RequestedModel        string         `json:"requested_model,omitempty"`
	UpstreamModel         *string        `json:"upstream_model,omitempty"`
	ChannelID             *int64         `json:"channel_id,omitempty"`
	ModelMappingChain     *string        `json:"model_mapping_chain,omitempty"`
	BillingTier           *string        `json:"billing_tier,omitempty"`
	BillingMode           *string        `json:"billing_mode,omitempty"`
	ServiceTier           *string        `json:"service_tier,omitempty"`
	ReasoningEffort       *string        `json:"reasoning_effort,omitempty"`
	InboundEndpoint       *string        `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint      *string        `json:"upstream_endpoint,omitempty"`
	GroupID               *int64         `json:"group_id,omitempty"`
	SubscriptionID        *int64         `json:"subscription_id,omitempty"`
	InputTokens           int            `json:"input_tokens"`
	OutputTokens          int            `json:"output_tokens"`
	CacheCreationTokens   int            `json:"cache_creation_tokens"`
	CacheReadTokens       int            `json:"cache_read_tokens"`
	CacheCreation5mTokens int            `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int            `json:"cache_creation_1h_tokens"`
	ImageOutputTokens     int            `json:"image_output_tokens"`
	ImageOutputCost       float64        `json:"image_output_cost"`
	InputCost             float64        `json:"input_cost"`
	OutputCost            float64        `json:"output_cost"`
	CacheCreationCost     float64        `json:"cache_creation_cost"`
	CacheReadCost         float64        `json:"cache_read_cost"`
	TotalCost             float64        `json:"total_cost"`
	ActualCost            float64        `json:"actual_cost"`
	RateMultiplier        float64        `json:"rate_multiplier"`
	PricingCurrency       *string        `json:"pricing_currency,omitempty"`
	AccountRateMultiplier *float64       `json:"account_rate_multiplier,omitempty"`
	BillingType           int8           `json:"billing_type"`
	RequestType           RequestType    `json:"request_type"`
	Stream                bool           `json:"stream"`
	OpenAIWSMode          bool           `json:"openai_ws_mode"`
	DurationMs            *int           `json:"duration_ms,omitempty"`
	FirstTokenMs          *int           `json:"first_token_ms,omitempty"`
	WSConnReused          *bool          `json:"ws_conn_reused,omitempty"`
	WSPreflightFailCount  *int           `json:"ws_preflight_fail_count,omitempty"`
	WSConnPickMs          *int           `json:"ws_conn_pick_ms,omitempty"`
	WSPayloadBytes        *int64         `json:"ws_payload_bytes,omitempty"`
	WSEventCount          *int           `json:"ws_event_count,omitempty"`
	WSQueueWaitMs         *int           `json:"ws_queue_wait_ms,omitempty"`
	CompactPayloadBytes   *int64         `json:"compact_payload_bytes,omitempty"`
	CompactRetryCount     *int           `json:"compact_retry_count,omitempty"`
	CompactClientCanceled *bool          `json:"compact_client_canceled,omitempty"`
	UserAgent             *string        `json:"user_agent,omitempty"`
	IPAddress             *string        `json:"ip_address,omitempty"`
	CacheTTLOverridden    bool           `json:"cache_ttl_overridden"`
	ImageCount            int            `json:"image_count"`
	ImageSize             *string        `json:"image_size,omitempty"`
	ImageInputSize        *string        `json:"image_input_size,omitempty"`
	ImageOutputSize       *string        `json:"image_output_size,omitempty"`
	ImageSizeSource       *string        `json:"image_size_source,omitempty"`
	ImageSizeBreakdown    map[string]int `json:"image_size_breakdown,omitempty"`
	MediaType             *string        `json:"media_type,omitempty"`
	VideoCount            int            `json:"video_count"`
	VideoResolution       *string        `json:"video_resolution,omitempty"`
	VideoDurationSeconds  *int           `json:"video_duration_seconds,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
}

func NewUsageLogSnapshot(log *UsageLog) UsageLogSnapshot {
	if log == nil {
		return UsageLogSnapshot{}
	}
	breakdown := make(map[string]int, len(log.ImageSizeBreakdown))
	for key, value := range log.ImageSizeBreakdown {
		breakdown[key] = value
	}
	return UsageLogSnapshot{
		UserID: log.UserID, APIKeyID: log.APIKeyID, AccountID: log.AccountID, RequestID: log.RequestID,
		Model: log.Model, RequestedModel: log.RequestedModel, UpstreamModel: log.UpstreamModel, ChannelID: log.ChannelID,
		ModelMappingChain: log.ModelMappingChain, BillingTier: log.BillingTier, BillingMode: log.BillingMode,
		ServiceTier: log.ServiceTier, ReasoningEffort: log.ReasoningEffort, InboundEndpoint: log.InboundEndpoint,
		UpstreamEndpoint: log.UpstreamEndpoint, GroupID: log.GroupID, SubscriptionID: log.SubscriptionID,
		InputTokens: log.InputTokens, OutputTokens: log.OutputTokens, CacheCreationTokens: log.CacheCreationTokens,
		CacheReadTokens: log.CacheReadTokens, CacheCreation5mTokens: log.CacheCreation5mTokens,
		CacheCreation1hTokens: log.CacheCreation1hTokens, ImageOutputTokens: log.ImageOutputTokens,
		ImageOutputCost: log.ImageOutputCost, InputCost: log.InputCost, OutputCost: log.OutputCost,
		CacheCreationCost: log.CacheCreationCost, CacheReadCost: log.CacheReadCost, TotalCost: log.TotalCost,
		ActualCost: log.ActualCost, RateMultiplier: log.RateMultiplier, PricingCurrency: log.PricingCurrency, AccountRateMultiplier: log.AccountRateMultiplier,
		BillingType: log.BillingType, RequestType: log.RequestType, Stream: log.Stream, OpenAIWSMode: log.OpenAIWSMode,
		DurationMs: log.DurationMs, FirstTokenMs: log.FirstTokenMs, WSConnReused: log.WSConnReused,
		WSPreflightFailCount: log.WSPreflightFailCount, WSConnPickMs: log.WSConnPickMs,
		WSPayloadBytes: log.WSPayloadBytes, WSEventCount: log.WSEventCount, WSQueueWaitMs: log.WSQueueWaitMs,
		CompactPayloadBytes: log.CompactPayloadBytes, CompactRetryCount: log.CompactRetryCount,
		CompactClientCanceled: log.CompactClientCanceled, UserAgent: log.UserAgent, IPAddress: log.IPAddress,
		CacheTTLOverridden: log.CacheTTLOverridden, ImageCount: log.ImageCount, ImageSize: log.ImageSize,
		ImageInputSize: log.ImageInputSize, ImageOutputSize: log.ImageOutputSize, ImageSizeSource: log.ImageSizeSource,
		ImageSizeBreakdown: breakdown, MediaType: log.MediaType, VideoCount: log.VideoCount,
		VideoResolution: log.VideoResolution, VideoDurationSeconds: log.VideoDurationSeconds, CreatedAt: log.CreatedAt,
	}
}

func (s UsageLogSnapshot) UsageLog() *UsageLog {
	breakdown := make(map[string]int, len(s.ImageSizeBreakdown))
	for key, value := range s.ImageSizeBreakdown {
		breakdown[key] = value
	}
	return &UsageLog{
		UserID: s.UserID, APIKeyID: s.APIKeyID, AccountID: s.AccountID, RequestID: s.RequestID,
		Model: s.Model, RequestedModel: s.RequestedModel, UpstreamModel: s.UpstreamModel, ChannelID: s.ChannelID,
		PricingPolicyID: s.ChannelID, ModelMappingChain: s.ModelMappingChain, BillingTier: s.BillingTier,
		BillingMode: s.BillingMode, ServiceTier: s.ServiceTier, ReasoningEffort: s.ReasoningEffort,
		InboundEndpoint: s.InboundEndpoint, UpstreamEndpoint: s.UpstreamEndpoint, GroupID: s.GroupID,
		SubscriptionID: s.SubscriptionID, InputTokens: s.InputTokens, OutputTokens: s.OutputTokens,
		CacheCreationTokens: s.CacheCreationTokens, CacheReadTokens: s.CacheReadTokens,
		CacheCreation5mTokens: s.CacheCreation5mTokens, CacheCreation1hTokens: s.CacheCreation1hTokens,
		ImageOutputTokens: s.ImageOutputTokens, ImageOutputCost: s.ImageOutputCost, InputCost: s.InputCost,
		OutputCost: s.OutputCost, CacheCreationCost: s.CacheCreationCost, CacheReadCost: s.CacheReadCost,
		TotalCost: s.TotalCost, ActualCost: s.ActualCost, RateMultiplier: s.RateMultiplier, PricingCurrency: s.PricingCurrency,
		AccountRateMultiplier: s.AccountRateMultiplier, BillingType: s.BillingType, RequestType: s.RequestType,
		Stream: s.Stream, OpenAIWSMode: s.OpenAIWSMode, DurationMs: s.DurationMs, FirstTokenMs: s.FirstTokenMs,
		WSConnReused: s.WSConnReused, WSPreflightFailCount: s.WSPreflightFailCount, WSConnPickMs: s.WSConnPickMs,
		WSPayloadBytes: s.WSPayloadBytes, WSEventCount: s.WSEventCount, WSQueueWaitMs: s.WSQueueWaitMs,
		CompactPayloadBytes: s.CompactPayloadBytes, CompactRetryCount: s.CompactRetryCount,
		CompactClientCanceled: s.CompactClientCanceled, UserAgent: s.UserAgent, IPAddress: s.IPAddress,
		CacheTTLOverridden: s.CacheTTLOverridden, ImageCount: s.ImageCount, ImageSize: s.ImageSize,
		ImageInputSize: s.ImageInputSize, ImageOutputSize: s.ImageOutputSize, ImageSizeSource: s.ImageSizeSource,
		ImageSizeBreakdown: breakdown, MediaType: s.MediaType, VideoCount: s.VideoCount,
		VideoResolution: s.VideoResolution, VideoDurationSeconds: s.VideoDurationSeconds, CreatedAt: s.CreatedAt,
	}
}

type UsageSettlementPayloadV1 struct {
	Version       int                  `json:"version"`
	Billing       UsageBillingSnapshot `json:"billing"`
	UsageLog      UsageLogSnapshot     `json:"usage_log"`
	QuotaPlatform string               `json:"quota_platform,omitempty"`
}

type UsageSettlementApplyResult struct {
	Billing          UsageBillingApplyResult
	UsageLogInserted bool
	AlreadySettled   bool
}

func NewUsageSettlementPayloadV1(command *UsageBillingCommand, log *UsageLog, quotaPlatform string) *UsageSettlementPayloadV1 {
	return &UsageSettlementPayloadV1{
		Version: UsageSettlementPayloadVersionV1, Billing: NewUsageBillingSnapshot(command),
		UsageLog: NewUsageLogSnapshot(log), QuotaPlatform: strings.TrimSpace(quotaPlatform),
	}
}

func (p *UsageSettlementPayloadV1) Validate(billingIdentity string, apiKeyID, userID, accountID int64) error {
	if p == nil {
		return ErrUsageSettlementPayloadRequired
	}
	if p.Version != UsageSettlementPayloadVersionV1 {
		return fmt.Errorf("unsupported usage settlement payload version: %d", p.Version)
	}
	billingIdentity = strings.TrimSpace(billingIdentity)
	if billingIdentity == "" || p.Billing.RequestID != billingIdentity || p.UsageLog.RequestID != billingIdentity {
		return errors.New("usage settlement payload billing identity mismatch")
	}
	if p.Billing.APIKeyID != apiKeyID || p.UsageLog.APIKeyID != apiKeyID ||
		p.Billing.UserID != userID || p.UsageLog.UserID != userID ||
		p.Billing.AccountID != accountID || p.UsageLog.AccountID != accountID {
		return errors.New("usage settlement payload subject mismatch")
	}
	if strings.TrimSpace(p.Billing.RequestFingerprint) == "" {
		return ErrUsageSettlementFingerprintRequired
	}
	return nil
}

func (p *UsageSettlementPayloadV1) CanonicalJSON(billingIdentity string, apiKeyID, userID, accountID int64) ([]byte, string, error) {
	if err := p.Validate(billingIdentity, apiKeyID, userID, accountID); err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return nil, "", err
	}
	if err := ValidateUsageSettlementPayloadJSON(encoded); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(sum[:]), nil
}

var forbiddenUsageSettlementPayloadKeys = map[string]struct{}{
	"credentials":   {},
	"credential":    {},
	"api_key":       {},
	"access_token":  {},
	"refresh_token": {},
	"password":      {},
	"secret":        {},
	"prompt":        {},
	"messages":      {},
	"input":         {},
	"output":        {},
	"request_body":  {},
	"response_body": {},
	"raw_request":   {},
	"raw_response":  {},
}

func ValidateUsageSettlementPayloadJSON(encoded []byte) error {
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return fmt.Errorf("decode usage settlement payload for sensitive-field audit: %w", err)
	}
	return auditUsageSettlementPayloadValue(value, "$")
}

func auditUsageSettlementPayloadValue(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if _, forbidden := forbiddenUsageSettlementPayloadKeys[normalized]; forbidden {
				return fmt.Errorf("%w: %s.%s", ErrUsageSettlementSensitivePayload, path, key)
			}
			if err := auditUsageSettlementPayloadValue(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := auditUsageSettlementPayloadValue(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}
