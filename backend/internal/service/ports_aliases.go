// P1 过渡别名（OpenSpec change refactor-architecture）：
// Repository/Cache 等端口接口下沉至 internal/ports，接口引用的领域类型下沉至
// internal/domain；service 保留同名别名/转发，调用点与 wire 装配零改动。
// 后续拆分（P2+）逐步改写调用点后收敛本文件。
package service

import (
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

type APIKeyCache = ports.APIKeyCache
type AccountQuotaWindowRepository = ports.AccountQuotaWindowRepository
type AffiliateRepository = ports.AffiliateRepository
type AnnouncementReadRepository = ports.AnnouncementReadRepository
type AnnouncementRepository = ports.AnnouncementRepository
type BillingCache = ports.BillingCache
type ContentModerationHashCache = ports.ContentModerationHashCache
type ContentModerationRepository = ports.ContentModerationRepository
type CyberSessionBlockStore = ports.CyberSessionBlockStore
type DBDumper = ports.DBDumper
type DashboardAggregationRepository = ports.DashboardAggregationRepository
type DashboardStatsCache = ports.DashboardStatsCache
type EmailCache = ports.EmailCache
type GatewayCache = ports.GatewayCache
type GeminiCliCodeAssistClient = ports.GeminiCliCodeAssistClient
type GeminiOAuthClient = ports.GeminiOAuthClient
type GeminiTokenCache = ports.GeminiTokenCache
type GitHubReleaseClient = ports.GitHubReleaseClient
type IdempotencyRepository = ports.IdempotencyRepository
type IdentityCache = ports.IdentityCache
type Internal500CounterCache = ports.Internal500CounterCache
type KimiDeviceSessionStore = ports.KimiDeviceSessionStore
type LeaderLockCache = ports.LeaderLockCache
type OpenAI403CounterCache = ports.OpenAI403CounterCache
type OpsRepository = ports.OpsRepository
type ProfitRepository = ports.ProfitRepository
type ProviderOAuthSessionStore = ports.ProviderOAuthSessionStore
type ProxyExitInfoProber = ports.ProxyExitInfoProber
type ProxyLatencyCache = ports.ProxyLatencyCache
type ProxyRepository = ports.ProxyRepository
type RPMCache = ports.RPMCache
type RedeemCache = ports.RedeemCache
type RedeemUserAdjustmentRepository = ports.RedeemUserAdjustmentRepository
type RefreshTokenCache = ports.RefreshTokenCache
type ScheduledTestPlanRepository = ports.ScheduledTestPlanRepository
type ScheduledTestResultRepository = ports.ScheduledTestResultRepository
type SchedulerOutboxCleanupLease = ports.SchedulerOutboxCleanupLease
type SchedulerOutboxRepository = ports.SchedulerOutboxRepository
type SecretEncryptor = ports.SecretEncryptor
type SessionLimitCache = ports.SessionLimitCache
type SettingRepository = ports.SettingRepository
type TempUnschedCache = ports.TempUnschedCache
type TimeoutCounterCache = ports.TimeoutCounterCache
type TotpCache = ports.TotpCache
type TurnstileVerifier = ports.TurnstileVerifier
type UsageCleanupRepository = ports.UsageCleanupRepository
type UserAttributeDefinitionRepository = ports.UserAttributeDefinitionRepository
type UserAttributeValueRepository = ports.UserAttributeValueRepository
type UserGroupRateRepository = ports.UserGroupRateRepository
type UserMsgQueueCache = ports.UserMsgQueueCache
type UserPlatformQuotaRepository = ports.UserPlatformQuotaRepository
type UserRPMCache = ports.UserRPMCache
