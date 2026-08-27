package billing

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type Provider string

const (
	ProviderGateway Provider = "gateway"
	ProviderOpenAI  Provider = "openai"
)

type PricingMissingPolicy string

const (
	PricingMissingLogZeroCost PricingMissingPolicy = "log_zero_cost"
)

type CostErrorPolicy string

const (
	CostErrorLogZeroAll         CostErrorPolicy = "log_zero_all"
	CostErrorLogZeroPricingOnly CostErrorPolicy = "log_zero_pricing_only"
)

type TokenBucketPolicy string

const (
	TokenBucketsGateway              TokenBucketPolicy = "gateway"
	TokenBucketsOpenAIExclusiveInput TokenBucketPolicy = "openai_exclusive_input"
)

// RecordPolicy names every platform-sensitive billing decision. Callers must
// select a policy explicitly instead of relying on zero-value platform guesses.
type RecordPolicy struct {
	Provider                   Provider
	PricingMissing             PricingMissingPolicy
	CostErrors                 CostErrorPolicy
	TokenBuckets               TokenBucketPolicy
	FixedLongContextThreshold  int
	FixedLongContextMultiplier float64
	OpenAILongContextEnabled   bool
}

// Usage 是跨平台结算使用的统一用量模型（design.md D5）。
// 各 provider 的 extractor 负责把协议事件映射到这些字段。
type Usage struct {
	InputTokens           int
	OutputTokens          int
	CacheCreationTokens   int
	CacheReadTokens       int
	CacheCreation5mTokens int
	CacheCreation1hTokens int
	ImageInputTokens      int
	ImageOutputTokens     int
}

// UsageTokens is the mutually exclusive token-bucket input consumed by the
// pricing engine. Provider extractors decide how protocol usage maps here.
type UsageTokens = Usage

// CostBreakdown is the canonical scalar result shared by calculation, usage
// logging, durable handoff, and legacy persistence.
type CostBreakdown struct {
	PricingCurrency   domain.PricingCurrency
	InputCost         float64
	OutputCost        float64
	ImageOutputCost   float64
	CacheCreationCost float64
	CacheReadCost     float64
	TotalCost         float64
	ActualCost        float64
	BillingMode       string
}

// RecordInput is the provider-neutral input consumed by the single settlement core.
type RecordInput struct {
	Policy                RecordPolicy
	RequestID             string
	BillingRequestID      string
	Model                 string
	RequestedModel        string
	UpstreamModel         string
	BillingModels         []string
	QuotaPlatform         string
	Usage                 Usage
	ForceCacheBilling     bool
	InboundEndpoint       string
	UpstreamEndpoint      string
	UserAgent             string
	IPAddress             string
	RequestPayloadHash    string
	ChannelID             int64
	ModelMappingChain     string
	RateMultiplier        float64
	ImageMultiplier       float64
	VideoMultiplier       float64
	AccountRateMultiplier float64
	BillingType           int8
	ServiceTier           *string
	ReasoningEffort       *string
	Stream                bool
	Duration              time.Duration
	FirstTokenMs          *int
	FirstOutputStages     *domain.FirstOutputStages
	ImageCount            int
	ImageSize             string
	ImageInputSize        string
	ImageOutputSize       string
	ImageSizeSource       string
	ImageSizeBreakdown    map[string]int
	VideoCount            int
	VideoUsage            bool
	VideoResolution       string
	VideoDurationSeconds  int
	CacheTTLOverridden    bool
	Compact               bool
	CompactPayloadBytes   *int64
	CompactRetryCount     int
	CompactClientCanceled bool
	CyberBlocked          bool
	OpenAIWSMode          bool
	WSPayloadBytes        *int64
	WSEventCount          *int
	WSConnReused          *bool
	WSConnPickMs          *int
	WSQueueWaitMs         *int
	Account               *domain.Account
	APIKey                *domain.APIKey
	User                  *domain.User
	Subscription          *domain.UserSubscription
}
