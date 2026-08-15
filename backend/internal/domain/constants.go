package domain

import "github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"

// Status constants
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusUnused   = "unused"
	StatusUsed     = "used"
	StatusExpired  = "expired"
)

// Role constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Platform constants
const (
	PlatformAnthropic   = "anthropic"
	PlatformOpenAI      = "openai"
	PlatformGemini      = "gemini"
	PlatformAntigravity = "antigravity"
	PlatformGrok        = "grok"
	PlatformKimi        = "kimi"
)

// Privacy mode markers stored on account extra.
const (
	PrivacyModeTrainingOff   = "training_off"
	PrivacyModeFailed        = "training_set_failed"
	PrivacyModeCFBlocked     = "training_set_cf_blocked"
	AntigravityPrivacySet    = "privacy_set"
	AntigravityPrivacyFailed = "privacy_set_failed"
)

type OpenAIImagesCapability string

const (
	OpenAIImagesCapabilityBasic  OpenAIImagesCapability = "images-basic"
	OpenAIImagesCapabilityNative OpenAIImagesCapability = "images-native"
)

// QuotaDimension constants for spark shadow accounts.
const (
	QuotaDimensionGlobal = "global"
	QuotaDimensionSpark  = "spark"
)

// Account type constants
const (
	AccountTypeOAuth          = "oauth"           // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken     = "setup-token"     // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey         = "apikey"          // API Key类型账号
	AccountTypeUpstream       = "upstream"        // 上游透传类型账号（通过 Base URL + API Key 连接上游）
	AccountTypeServiceAccount = "service_account" // Google Service Account 类型账号（用于 Vertex AI）
)

// Redeem type constants
const (
	RedeemTypeBalance      = "balance"
	RedeemTypeConcurrency  = "concurrency"
	RedeemTypeSubscription = "subscription"
	RedeemTypeInvitation   = "invitation"
)

// PromoCode status constants
const (
	PromoCodeStatusActive   = "active"
	PromoCodeStatusDisabled = "disabled"
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = "admin_balance"     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = "admin_concurrency" // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = "standard"     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = "subscription" // 订阅模式（按限额控制）
)

// Subscription status constants
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusSuspended = "suspended"
)

// AntigravityGemini31ProAgentModel is the upstream route for Gemini 3.1 Pro High.
const AntigravityGemini31ProAgentModel = "gemini-pro-agent"

// DefaultAntigravityModelMapping 是 Antigravity 平台的默认模型映射
// 当账号未配置 model_mapping 时使用此默认值（由 modelcatalog JSON 填充）。
var DefaultAntigravityModelMapping map[string]string

func init() {
	DefaultAntigravityModelMapping = modelcatalog.AntigravityDefaultMapping()
}

func CurrentDefaultAntigravityModelMapping() map[string]string {
	return modelcatalog.AntigravityDefaultMapping()
}
