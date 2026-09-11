// P1 过渡别名（OpenSpec change refactor-architecture）：
// Repository/Cache 等端口接口下沉至 internal/ports，接口引用的领域类型下沉至
// internal/domain；service 保留同名别名/转发，调用点与 wire 装配零改动。
// 后续拆分（P2+）逐步改写调用点后收敛本文件。
package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type APIKeyAuthCacheEntry = domain.APIKeyAuthCacheEntry
type APIKeyAuthGroupSnapshot = domain.APIKeyAuthGroupSnapshot
type APIKeyAuthSnapshot = domain.APIKeyAuthSnapshot
type APIKeyAuthUserSnapshot = domain.APIKeyAuthUserSnapshot
type APIKeyListFilters = domain.APIKeyListFilters
type APIKeyQuotaUsageState = domain.APIKeyQuotaUsageState
type APIKeyRateLimitCacheData = domain.APIKeyRateLimitCacheData
type APIKeyRateLimitData = domain.APIKeyRateLimitData
type AccountBulkUpdate = domain.AccountBulkUpdate
type AccountCostConfig = domain.AccountCostConfig

const AccountListGroupUngrouped = domain.AccountListGroupUngrouped
const AccountListStatusBanned = domain.AccountListStatusBanned
const AccountListStatusDeleted = domain.AccountListStatusDeleted
const AccountListStatusTrash = domain.AccountListStatusTrash
const AccountPrivacyModeUnsetFilter = domain.AccountPrivacyModeUnsetFilter

type AccountQuotaState = domain.AccountQuotaState
type AccountQuotaUsageObservation = domain.AccountQuotaUsageObservation
type AccountQuotaWindow = domain.AccountQuotaWindow
type AccountSubscriptionCycle = domain.AccountSubscriptionCycle
type AccountSubscriptionLossSummary = domain.AccountSubscriptionLossSummary
type AccountSubscriptionRefund = domain.AccountSubscriptionRefund
type AccountSubscriptionTermination = domain.AccountSubscriptionTermination
type AffiliateAdminEntry = domain.AffiliateAdminEntry
type AffiliateAdminFilter = domain.AffiliateAdminFilter
type AffiliateInviteRecord = domain.AffiliateInviteRecord
type AffiliateInvitee = domain.AffiliateInvitee
type AffiliateRebateRecord = domain.AffiliateRebateRecord
type AffiliateRecordFilter = domain.AffiliateRecordFilter
type AffiliateSummary = domain.AffiliateSummary
type AffiliateTransferRecord = domain.AffiliateTransferRecord
type AffiliateUserOverview = domain.AffiliateUserOverview
type AnnouncementListFilters = domain.AnnouncementListFilters
type BackupObjectStore = domain.BackupObjectStore
type BackupObjectStoreFactory = domain.BackupObjectStoreFactory
type BackupS3Config = domain.BackupS3Config
type BatchImageBalanceHoldCommand = domain.BatchImageBalanceHoldCommand
type BatchImageBalanceHoldResult = domain.BatchImageBalanceHoldResult
type BatchImageCounts = domain.BatchImageCounts
type BatchImageItem = domain.BatchImageItem
type BatchImageItemFilter = domain.BatchImageItemFilter
type BatchImageJob = domain.BatchImageJob
type BatchImageJobFilter = domain.BatchImageJobFilter

const BatchImageJobStatusCancelled = domain.BatchImageJobStatusCancelled
const BatchImageJobStatusCompleted = domain.BatchImageJobStatusCompleted
const BatchImageJobStatusCreated = domain.BatchImageJobStatusCreated
const BatchImageJobStatusFailed = domain.BatchImageJobStatusFailed
const BatchImageJobStatusIndexing = domain.BatchImageJobStatusIndexing
const BatchImageJobStatusOutputDeleted = domain.BatchImageJobStatusOutputDeleted
const BatchImageJobStatusRunning = domain.BatchImageJobStatusRunning
const BatchImageJobStatusSettling = domain.BatchImageJobStatusSettling
const BatchImageJobStatusSubmitted = domain.BatchImageJobStatusSubmitted
const BatchImageJobStatusUploading = domain.BatchImageJobStatusUploading
const BatchImageProviderVertex = domain.BatchImageProviderVertex

type BatchImageTransitionOptions = domain.BatchImageTransitionOptions
type BillingMode = domain.BillingMode

const BillingModeImage = domain.BillingModeImage
const BillingModePerRequest = domain.BillingModePerRequest
const BillingModeToken = domain.BillingModeToken
const BillingModeVideo = domain.BillingModeVideo

type ChannelModelPricing = domain.ChannelModelPricing
type ClaudeUsageResponse = domain.ClaudeUsageResponse
type ClaudeUsageWindow = domain.ClaudeUsageWindow
type ContentModerationCleanupResult = domain.ContentModerationCleanupResult
type ContentModerationElevatedUserCacheEntry = domain.ContentModerationElevatedUserCacheEntry
type ContentModerationLog = domain.ContentModerationLog
type ContentModerationLogFilter = domain.ContentModerationLogFilter
type ContentModerationLogSummary = domain.ContentModerationLogSummary
type CreateBatchImageItemParams = domain.CreateBatchImageItemParams
type CreateBatchImageJobParams = domain.CreateBatchImageJobParams
type DeletedKeyAuditResult = domain.DeletedKeyAuditResult

const DingTalkConnectSyntheticEmailDomain = domain.DingTalkConnectSyntheticEmailDomain

var ErrAPIKeyExists = domain.ErrAPIKeyExists
var ErrAPIKeyNotFound = domain.ErrAPIKeyNotFound
var ErrAccountNilInput = domain.ErrAccountNilInput
var ErrAccountNotFound = domain.ErrAccountNotFound
var ErrAccountNotInFallback = domain.ErrAccountNotInFallback
var ErrAccountNotSchedulable = domain.ErrAccountNotSchedulable
var ErrRetiredBedrockAccount = domain.ErrRetiredBedrockAccount
var ErrVertexServiceAccountOnboardingDisabled = domain.ErrVertexServiceAccountOnboardingDisabled
var ErrAffiliateCodeInvalid = domain.ErrAffiliateCodeInvalid
var ErrAffiliateCodeTaken = domain.ErrAffiliateCodeTaken
var ErrAffiliateProfileNotFound = domain.ErrAffiliateProfileNotFound
var ErrAffiliateQuotaEmpty = domain.ErrAffiliateQuotaEmpty
var ErrAttributeDefinitionNotFound = domain.ErrAttributeDefinitionNotFound
var ErrAttributeKeyExists = domain.ErrAttributeKeyExists
var ErrBatchImageAlreadyQueued = domain.ErrBatchImageAlreadyQueued
var ErrBatchImageAlreadySettled = domain.ErrBatchImageAlreadySettled
var ErrBatchImageDownloadLimited = domain.ErrBatchImageDownloadLimited
var ErrBatchImageIndexStateConflict = domain.ErrBatchImageIndexStateConflict
var ErrBatchImageInsufficientBalance = domain.ErrBatchImageInsufficientBalance
var ErrBatchImageInvalidProvider = domain.ErrBatchImageInvalidProvider
var ErrBatchImageInvalidTransition = domain.ErrBatchImageInvalidTransition
var ErrBatchImageItemExists = domain.ErrBatchImageItemExists
var ErrBatchImageItemNotFound = domain.ErrBatchImageItemNotFound
var ErrBatchImageJobExists = domain.ErrBatchImageJobExists
var ErrBatchImageJobNotFound = domain.ErrBatchImageJobNotFound
var ErrBatchImageQueueEmpty = domain.ErrBatchImageQueueEmpty
var ErrBatchImageRecordDeleteNotReady = domain.ErrBatchImageRecordDeleteNotReady
var ErrBatchImageSettlementCostExceedsHold = domain.ErrBatchImageSettlementCostExceedsHold
var ErrBatchImageSettlementInvalidStatus = domain.ErrBatchImageSettlementInvalidStatus
var ErrBatchImageSettlementManifestConflict = domain.ErrBatchImageSettlementManifestConflict
var ErrChannelExists = domain.ErrChannelExists
var ErrChannelNotFound = domain.ErrChannelNotFound
var ErrDashboardStatsCacheMiss = domain.ErrDashboardStatsCacheMiss
var ErrGroupExists = domain.ErrGroupExists
var ErrGroupNotFound = domain.ErrGroupNotFound
var ErrIdentityProviderInvalid = domain.ErrIdentityProviderInvalid
var ErrInvalidBatchImageQueuePayload = domain.ErrInvalidBatchImageQueuePayload
var ErrOpsPreaggregatedNotPopulated = domain.ErrOpsPreaggregatedNotPopulated
var ErrProxyNotFound = domain.ErrProxyNotFound
var ErrRedeemCodeNotFound = domain.ErrRedeemCodeNotFound
var ErrRedeemCodeUsed = domain.ErrRedeemCodeUsed
var ErrRefreshTokenNotFound = domain.ErrRefreshTokenNotFound
var ErrSettingNotFound = domain.ErrSettingNotFound
var ErrSubscriptionAlreadyExists = domain.ErrSubscriptionAlreadyExists
var ErrSubscriptionCycleAlreadyTerminated = domain.ErrSubscriptionCycleAlreadyTerminated
var ErrSubscriptionCycleNotFound = domain.ErrSubscriptionCycleNotFound
var ErrSubscriptionCycleSettled = domain.ErrSubscriptionCycleSettled
var ErrSubscriptionNilInput = domain.ErrSubscriptionNilInput
var ErrSubscriptionNotFound = domain.ErrSubscriptionNotFound
var ErrSubscriptionRefundExceedsFee = domain.ErrSubscriptionRefundExceedsFee
var ErrSubscriptionRefundNotFound = domain.ErrSubscriptionRefundNotFound
var ErrSubscriptionRefundVoided = domain.ErrSubscriptionRefundVoided
var ErrSubscriptionRestoreConflict = domain.ErrSubscriptionRestoreConflict
var ErrSubscriptionTerminationNotFound = domain.ErrSubscriptionTerminationNotFound
var ErrSubscriptionTerminationReversed = domain.ErrSubscriptionTerminationReversed
var ErrUpstreamBillingProbeIdentityChanged = domain.ErrUpstreamBillingProbeIdentityChanged
var ErrUsageBillingRequestConflict = domain.ErrUsageBillingRequestConflict
var ErrUsageBillingRequestIDRequired = domain.ErrUsageBillingRequestIDRequired
var ErrUsageLogNotFound = domain.ErrUsageLogNotFound
var ErrUserNotFound = domain.ErrUserNotFound
var ErrUserPlatformQuotaFKViolation = domain.ErrUserPlatformQuotaFKViolation
var ErrUserPlatformQuotaNotFound = domain.ErrUserPlatformQuotaNotFound

const FallbackModeDirect = domain.FallbackModeDirect
const FallbackModeNone = domain.FallbackModeNone
const FallbackModeProxy = domain.FallbackModeProxy

type Fingerprint = domain.Fingerprint
type GeminiTokenResponse = domain.GeminiTokenResponse
type GeminiUsageTotals = domain.GeminiUsageTotals
type GitHubRelease = domain.GitHubRelease
type GroupAccountCapacityRow = domain.GroupAccountCapacityRow
type GroupRPMOverrideInput = domain.GroupRPMOverrideInput
type GroupRateMultiplierInput = domain.GroupRateMultiplierInput
type GroupSortOrderUpdate = domain.GroupSortOrderUpdate
type GroupTrafficAvailability = domain.GroupTrafficAvailability
type HTTPUpstreamProfile = domain.HTTPUpstreamProfile

const HTTPUpstreamProfileDefault = domain.HTTPUpstreamProfileDefault
const HTTPUpstreamProfileGrok = domain.HTTPUpstreamProfileGrok
const HTTPUpstreamProfileOpenAI = domain.HTTPUpstreamProfileOpenAI

type IdempotencyRecord = domain.IdempotencyRecord

const IdempotencyStatusFailedRetryable = domain.IdempotencyStatusFailedRetryable
const IdempotencyStatusProcessing = domain.IdempotencyStatusProcessing
const IdempotencyStatusSucceeded = domain.IdempotencyStatusSucceeded

type KimiDevicePollLease = domain.KimiDevicePollLease
type KimiDeviceSession = domain.KimiDeviceSession
type KimiOAuthTokenInfo = domain.KimiOAuthTokenInfo

const LinuxDoConnectSyntheticEmailDomain = domain.LinuxDoConnectSyntheticEmailDomain

type MarkBatchImageJobSettledParams = domain.MarkBatchImageJobSettledParams
type NotifyEmailEntry = domain.NotifyEmailEntry
type NullableInt64Update = domain.NullableInt64Update
type NullableTimeUpdate = domain.NullableTimeUpdate

const OIDCConnectSyntheticEmailDomain = domain.OIDCConnectSyntheticEmailDomain

type OpsAlertEvent = domain.OpsAlertEvent
type OpsAlertEventFilter = domain.OpsAlertEventFilter
type OpsAlertRule = domain.OpsAlertRule
type OpsAlertSilence = domain.OpsAlertSilence

const OpsAlertStatusFiring = domain.OpsAlertStatusFiring
const OpsAlertStatusManualResolved = domain.OpsAlertStatusManualResolved
const OpsAlertStatusResolved = domain.OpsAlertStatusResolved

type OpsDashboardFilter = domain.OpsDashboardFilter
type OpsDashboardOverview = domain.OpsDashboardOverview
type OpsErrorDistributionItem = domain.OpsErrorDistributionItem
type OpsErrorDistributionResponse = domain.OpsErrorDistributionResponse
type OpsErrorLog = domain.OpsErrorLog
type OpsErrorLogDetail = domain.OpsErrorLogDetail
type OpsErrorLogFilter = domain.OpsErrorLogFilter
type OpsErrorLogList = domain.OpsErrorLogList
type OpsErrorTrendPoint = domain.OpsErrorTrendPoint
type OpsErrorTrendResponse = domain.OpsErrorTrendResponse
type OpsInsertErrorLogInput = domain.OpsInsertErrorLogInput
type OpsInsertSystemLogInput = domain.OpsInsertSystemLogInput
type OpsInsertSystemMetricsInput = domain.OpsInsertSystemMetricsInput
type OpsJobHeartbeat = domain.OpsJobHeartbeat
type OpsLatencyHistogramBucket = domain.OpsLatencyHistogramBucket
type OpsLatencyHistogramResponse = domain.OpsLatencyHistogramResponse
type OpsOpenAITokenStatsFilter = domain.OpsOpenAITokenStatsFilter
type OpsOpenAITokenStatsItem = domain.OpsOpenAITokenStatsItem
type OpsOpenAITokenStatsResponse = domain.OpsOpenAITokenStatsResponse
type OpsPercentiles = domain.OpsPercentiles
type OpsQueryMode = domain.OpsQueryMode

const OpsQueryModeAuto = domain.OpsQueryModeAuto
const OpsQueryModePreagg = domain.OpsQueryModePreagg
const OpsQueryModeRaw = domain.OpsQueryModeRaw

type OpsRateSummary = domain.OpsRateSummary
type OpsRealtimeTrafficSummary = domain.OpsRealtimeTrafficSummary
type OpsRequestDetail = domain.OpsRequestDetail
type OpsRequestDetailFilter = domain.OpsRequestDetailFilter
type OpsRequestKind = domain.OpsRequestKind

const OpsRequestKindError = domain.OpsRequestKindError
const OpsRequestKindSuccess = domain.OpsRequestKindSuccess

type OpsSystemLog = domain.OpsSystemLog
type OpsSystemLogCleanupAudit = domain.OpsSystemLogCleanupAudit
type OpsSystemLogCleanupFilter = domain.OpsSystemLogCleanupFilter
type OpsSystemLogFilter = domain.OpsSystemLogFilter
type OpsSystemLogList = domain.OpsSystemLogList
type OpsSystemMetricsSnapshot = domain.OpsSystemMetricsSnapshot
type OpsThroughputGroupBreakdownItem = domain.OpsThroughputGroupBreakdownItem
type OpsThroughputPlatformBreakdownItem = domain.OpsThroughputPlatformBreakdownItem
type OpsThroughputTrendPoint = domain.OpsThroughputTrendPoint
type OpsThroughputTrendResponse = domain.OpsThroughputTrendResponse
type OpsUpsertJobHeartbeatInput = domain.OpsUpsertJobHeartbeatInput
type OpsUpstreamErrorEvent = domain.OpsUpstreamErrorEvent
type OpsWindowStats = domain.OpsWindowStats
type PasswordResetTokenData = domain.PasswordResetTokenData
type PendingOAuthBindLoginSession = domain.PendingOAuthBindLoginSession
type PricingInterval = domain.PricingInterval
type ProfitAccountDailyUsagePoint = domain.ProfitAccountDailyUsagePoint
type ProfitAccountUsageRange = domain.ProfitAccountUsageRange
type ProfitDailyUsagePoint = domain.ProfitDailyUsagePoint
type ProfitUsageStats = domain.ProfitUsageStats

const ProviderOAuthFlowAuthorizationCode = domain.ProviderOAuthFlowAuthorizationCode
const ProviderOAuthFlowDeviceCode = domain.ProviderOAuthFlowDeviceCode

type ProviderOAuthSession = domain.ProviderOAuthSession

const ProviderOAuthSessionAuthorized = domain.ProviderOAuthSessionAuthorized
const ProviderOAuthSessionCancelled = domain.ProviderOAuthSessionCancelled
const ProviderOAuthSessionDenied = domain.ProviderOAuthSessionDenied

type ProviderOAuthSessionLease = domain.ProviderOAuthSessionLease

const ProviderOAuthSessionPending = domain.ProviderOAuthSessionPending
const ProviderStatusFresh = domain.ProviderStatusFresh

type ProviderStatusHistoryFilter = domain.ProviderStatusHistoryFilter

const ProviderStatusOpenAI = domain.ProviderStatusOpenAI

type ProviderStatusSnapshotRecord = domain.ProviderStatusSnapshotRecord

const ProviderStatusStale = domain.ProviderStatusStale
const ProviderStatusUnavailable = domain.ProviderStatusUnavailable

type Proxy = domain.Proxy
type ProxyAccountSummary = domain.ProxyAccountSummary
type ProxyExitInfo = domain.ProxyExitInfo
type ProxyLatencyInfo = domain.ProxyLatencyInfo
type ProxyWithAccountCount = domain.ProxyWithAccountCount

const QuotaWindowCloseCleared = domain.QuotaWindowCloseCleared
const QuotaWindowCloseObserved = domain.QuotaWindowCloseObserved
const QuotaWindowCloseReplaced = domain.QuotaWindowCloseReplaced
const QuotaWindowCloseResetCard = domain.QuotaWindowCloseResetCard
const QuotaWindowSourceObserved = domain.QuotaWindowSourceObserved
const QuotaWindowSourceResetCard = domain.QuotaWindowSourceResetCard
const QuotaWindowSourceSeed = domain.QuotaWindowSourceSeed
const RateLimitWindow1d = domain.RateLimitWindow1d
const RateLimitWindow5h = domain.RateLimitWindow5h
const RateLimitWindow7d = domain.RateLimitWindow7d

type RedeemCodeBatchUpdateFields = domain.RedeemCodeBatchUpdateFields
type RefreshTokenData = domain.RefreshTokenData
type RequestType = domain.RequestType

const RequestTypeCompact = domain.RequestTypeCompact
const RequestTypeCyberBlocked = domain.RequestTypeCyberBlocked
const RequestTypeStream = domain.RequestTypeStream
const RequestTypeSync = domain.RequestTypeSync
const RequestTypeUnknown = domain.RequestTypeUnknown
const RequestTypeWSV2 = domain.RequestTypeWSV2

type ReservedBatchImageJob = domain.ReservedBatchImageJob
type ScheduledDiagnosticsStatus = domain.ScheduledDiagnosticsStatus
type ScheduledTestPlan = domain.ScheduledTestPlan
type ScheduledTestResult = domain.ScheduledTestResult
type SchedulerBucket = domain.SchedulerBucket
type SchedulerOutboxEvent = domain.SchedulerOutboxEvent

const SchedulerOutboxEventAccountBulkChanged = domain.SchedulerOutboxEventAccountBulkChanged
const SchedulerOutboxEventAccountChanged = domain.SchedulerOutboxEventAccountChanged
const SchedulerOutboxEventAccountGroupsChanged = domain.SchedulerOutboxEventAccountGroupsChanged
const SchedulerOutboxEventAccountLastUsed = domain.SchedulerOutboxEventAccountLastUsed
const SchedulerOutboxEventFullRebuild = domain.SchedulerOutboxEventFullRebuild
const SchedulerOutboxEventGroupChanged = domain.SchedulerOutboxEventGroupChanged

type Setting = domain.Setting

const StatusAPIKeyActive = domain.StatusAPIKeyActive
const StatusAPIKeyDisabled = domain.StatusAPIKeyDisabled
const StatusAPIKeyExpired = domain.StatusAPIKeyExpired
const StatusAPIKeyQuotaExhausted = domain.StatusAPIKeyQuotaExhausted

type SubscriptionCacheData = domain.SubscriptionCacheData
type SubscriptionCycleUsageRange = domain.SubscriptionCycleUsageRange

const SubscriptionStatusRevoked = domain.SubscriptionStatusRevoked

type SubscriptionTerminationWriteResult = domain.SubscriptionTerminationWriteResult
type TempUnschedState = domain.TempUnschedState
type TotpLoginSession = domain.TotpLoginSession
type TotpSetupSession = domain.TotpSetupSession
type TrafficAvailability = domain.TrafficAvailability
type TrafficAvailabilityBucket = domain.TrafficAvailabilityBucket
type TurnstileVerifyResponse = domain.TurnstileVerifyResponse
type UpdateBatchImageJobProviderSubmitParams = domain.UpdateBatchImageJobProviderSubmitParams
type UpdateUserAttributeInput = domain.UpdateUserAttributeInput
type UpsertUserAvatarInput = domain.UpsertUserAvatarInput

const UpstreamBillingProbeEnabledExtraKey = domain.UpstreamBillingProbeEnabledExtraKey
const UpstreamBillingProbeExtraKey = domain.UpstreamBillingProbeExtraKey

type UpstreamBillingProbeSnapshot = domain.UpstreamBillingProbeSnapshot

const UpstreamBillingProbeStatusFailed = domain.UpstreamBillingProbeStatusFailed
const UpstreamBillingProbeStatusOK = domain.UpstreamBillingProbeStatusOK
const UpstreamBillingProbeStatusUnsupported = domain.UpstreamBillingProbeStatusUnsupported
const UpstreamBillingRateSyncEnabledExtraKey = domain.UpstreamBillingRateSyncEnabledExtraKey

type UsageBillingApplyResult = domain.UsageBillingApplyResult
type UsageCleanupFilters = domain.UsageCleanupFilters

const UsageCleanupStatusCanceled = domain.UsageCleanupStatusCanceled
const UsageCleanupStatusFailed = domain.UsageCleanupStatusFailed
const UsageCleanupStatusPending = domain.UsageCleanupStatusPending
const UsageCleanupStatusRunning = domain.UsageCleanupStatusRunning
const UsageCleanupStatusSucceeded = domain.UsageCleanupStatusSucceeded

type UsageCleanupTask = domain.UsageCleanupTask
type UsageLogCreateError = domain.UsageLogCreateError
type UserAttributeDefinition = domain.UserAttributeDefinition
type UserAttributeOption = domain.UserAttributeOption
type UserAttributeType = domain.UserAttributeType
type UserAttributeValidation = domain.UserAttributeValidation
type UserAttributeValue = domain.UserAttributeValue
type UserAuthIdentityRecord = domain.UserAuthIdentityRecord
type UserAvatar = domain.UserAvatar
type UserGroupRateEntry = domain.UserGroupRateEntry
type UserListFilters = domain.UserListFilters
type UserPlatformQuotaCacheEntry = domain.UserPlatformQuotaCacheEntry

const UserPlatformQuotaCacheSchemaV1 = domain.UserPlatformQuotaCacheSchemaV1

type UserPlatformQuotaKey = domain.UserPlatformQuotaKey
type UserPlatformQuotaRecord = domain.UserPlatformQuotaRecord
type UserPlatformQuotaSnapshot = domain.UserPlatformQuotaSnapshot
type VerificationCodeData = domain.VerificationCodeData

const WeChatConnectSyntheticEmailDomain = domain.WeChatConnectSyntheticEmailDomain

func ApplyLegacyRequestFields(requestType RequestType, fallbackStream bool, fallbackOpenAIWSMode bool) (bool, bool) {
	return domain.ApplyLegacyRequestFields(requestType, fallbackStream, fallbackOpenAIWSMode)
}
func BatchImageCaptureRequestID(batchID string) string {
	return domain.BatchImageCaptureRequestID(batchID)
}
func BatchImageHoldRequestID(batchID string) string { return domain.BatchImageHoldRequestID(batchID) }
func BatchImageReleaseRequestID(batchID string) string {
	return domain.BatchImageReleaseRequestID(batchID)
}
func CanTransitionBatchImageJob(from string, to string) bool {
	return domain.CanTransitionBatchImageJob(from, to)
}
func FindMatchingInterval(intervals []PricingInterval, totalTokens int) *PricingInterval {
	return domain.FindMatchingInterval(intervals, totalTokens)
}
func HTTPUpstreamProfileFromContext(ctx context.Context) HTTPUpstreamProfile {
	return domain.HTTPUpstreamProfileFromContext(ctx)
}
func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	return domain.HTTPUpstreamRedirectsDisabled(ctx)
}
func IsSupportedBatchImageProvider(provider string) bool {
	return domain.IsSupportedBatchImageProvider(provider)
}
func IsTerminalBatchImageJobStatus(status string) bool {
	return domain.IsTerminalBatchImageJobStatus(status)
}
func IsUsageLogCreateDropped(err error) bool      { return domain.IsUsageLogCreateDropped(err) }
func IsUsageLogCreateNotPersisted(err error) bool { return domain.IsUsageLogCreateNotPersisted(err) }
func IsValidBatchImageID(batchID string) bool     { return domain.IsValidBatchImageID(batchID) }
func IsWindowExpired(windowStart *time.Time, duration time.Duration) bool {
	return domain.IsWindowExpired(windowStart, duration)
}
func MarkUsageLogCreateDropped(err error) error { return domain.MarkUsageLogCreateDropped(err) }
func MarkUsageLogCreateNotPersisted(err error) error {
	return domain.MarkUsageLogCreateNotPersisted(err)
}
func MarshalNotifyEmails(entries []NotifyEmailEntry) string {
	return domain.MarshalNotifyEmails(entries)
}
func NewBatchImageID() (string, error)                { return domain.NewBatchImageID() }
func ParseNotifyEmails(raw string) []NotifyEmailEntry { return domain.ParseNotifyEmails(raw) }
func ParseSchedulerBucket(raw string) (SchedulerBucket, bool) {
	return domain.ParseSchedulerBucket(raw)
}
func RequestTypeFromInt16(v int16) RequestType { return domain.RequestTypeFromInt16(v) }
func ResolveProxyFallbackTarget(start Proxy, byID map[int64]Proxy, now time.Time) (*int64, bool) {
	return domain.ResolveProxyFallbackTarget(start, byID, now)
}
func ShouldBillAfterUsageLogCreate(inserted bool, err error) bool {
	return domain.ShouldBillAfterUsageLogCreate(inserted, err)
}
func WithHTTPUpstreamProfile(ctx context.Context, profile HTTPUpstreamProfile) context.Context {
	return domain.WithHTTPUpstreamProfile(ctx, profile)
}
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	return domain.WithHTTPUpstreamRedirectsDisabled(ctx)
}
