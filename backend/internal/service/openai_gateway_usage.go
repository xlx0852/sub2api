package service

// 本文件由 openai_gateway_service.go 纯移动拆分而来：用量记录、计费成本计算与
// Codex 用量快照。仅做代码搬迁，无任何行为变更。

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	appbilling "github.com/Wei-Shaw/sub2api/internal/app/billing"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"go.uber.org/zap"
)

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result             *OpenAIForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string // 请求的 User-Agent
	IPAddress          string // 请求的客户端 IP 地址
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
	QuotaPlatform      string // user×platform quota platform resolved by the handler before async billing.
	// CyberBlocked 为 true 时把该用量行标记为 cyber（request_type=cyber），计费逻辑不变。
	CyberBlocked bool
	ChannelUsageFields
}

// CyberPolicyUsageInput 是 cyber 拒绝、未走正常 RecordUsage 的请求记录用量的入参。
// 用量按上游真实 token 计费，与 WS cyber 及正常请求口径一致（InputTokens/OutputTokens
// 取自上游 response.failed 报告的 usage，即 mark.UpstreamInTok/OutTok）。
type CyberPolicyUsageInput struct {
	APIKey       *APIKey
	Account      *Account
	Subscription *UserSubscription
	RequestID    string
	Model        string
	Stream       bool
	InputTokens  int
	OutputTokens int
	// 渠道归因与请求级 meta，使 cyber 计费行与正常 RecordUsage 行口径一致
	// （否则 cyber 行 channel_id 等为空，渠道维度统计会遗漏 cyber 命中）。
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
	ChannelUsageFields
}

// RecordCyberPolicyUsageLog 为被上游 cyber_policy 拒绝、未走正常 RecordUsage 的请求
// （HTTP forward 返回错误路径）记录用量并按上游真实 token 计费，使其与 WS cyber 路径、
// 与正常请求的计费口径统一（不再是 tokens=0 免费行）。token 取自上游 response.failed
// 报告的 usage（非流式直接拒通常为 0，cost 随之为 0）。复用 RecordUsage 完成成本计算、
// 扣费与用量行写入（request_type=cyber 由 CyberBlocked 置位）。仅 forward 返回错误的
// 路径由 handler 调用，避免与成功路径的正常 RecordUsage 重复。
func (s *OpenAIGatewayService) RecordCyberPolicyUsageLog(ctx context.Context, in CyberPolicyUsageInput) {
	if s == nil || in.APIKey == nil || in.APIKey.User == nil || in.Account == nil || strings.TrimSpace(in.Model) == "" {
		return
	}
	result := &OpenAIForwardResult{
		RequestID: in.RequestID,
		Model:     in.Model,
		Stream:    in.Stream,
		Usage: OpenAIUsage{
			InputTokens:  in.InputTokens,
			OutputTokens: in.OutputTokens,
		},
	}
	if err := s.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result:             result,
		APIKey:             in.APIKey,
		User:               in.APIKey.User,
		Account:            in.Account,
		Subscription:       in.Subscription,
		InboundEndpoint:    in.InboundEndpoint,
		UpstreamEndpoint:   in.UpstreamEndpoint,
		UserAgent:          in.UserAgent,
		IPAddress:          in.IPAddress,
		RequestPayloadHash: in.RequestPayloadHash,
		APIKeyService:      in.APIKeyService,
		ChannelUsageFields: in.ChannelUsageFields,
		CyberBlocked:       true,
	}); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber usage record failed: request_id=%s err=%v", in.RequestID, err)
	}
}

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	if input == nil {
		return errors.New("openai usage input is nil")
	}
	result := input.Result
	if result == nil {
		return errors.New("openai usage result is nil")
	}
	if s.rateLimitService != nil && input.Account != nil && input.Account.Platform == PlatformOpenAI {
		s.rateLimitService.ResetOpenAI403Counter(ctx, input.Account.ID)
	}

	apiKey := input.APIKey
	user := input.User
	account := input.Account
	// 用户订阅配额模式已退役。保留输入字段只为兼容现有调用方；
	// 新用量统一余额扣费且不再写入 subscription_id。
	var subscription *UserSubscription
	ApplyOpenAIImageBillingResolution(result)
	normalizeOpenAIUsageForBilling(&result.Usage)

	// OpenAI input_tokens 是总输入，包含缓存读取和缓存写入明细。
	// 将三类 token 拆成互斥桶，避免缓存写入同时按普通输入和 cache_write 重复计费。
	actualInputTokens := result.Usage.InputTokens - result.Usage.CacheReadInputTokens - result.Usage.CacheCreationInputTokens
	if actualInputTokens < 0 {
		actualInputTokens = 0
	}

	// Calculate cost
	tokens := UsageTokens{
		InputTokens:         actualInputTokens,
		ImageInputTokens:    result.Usage.ImageInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
		ImageOutputTokens:   result.Usage.ImageOutputTokens,
	}

	// Get rate multiplier
	multiplier := resolveUsageRateMultiplier(ctx, s.cfg, s.userGroupRateResolver, user.ID, apiKey, func() *userGroupRateResolver {
		return newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_gateway")
	})
	// token 倍率叠加高峰因子（token 计费含图片 token，图片按次倍率不受影响）。高峰因子按请求时刻现算，
	// 不并入上面的 Resolve，以免污染 user:group 倍率缓存。
	multiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, multiplier, timezone.Now())
	videoMultiplier := resolveVideoRateMultiplier(apiKey, multiplier)

	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if result.BillingModel != "" {
		billingModel = strings.TrimSpace(result.BillingModel)
	}
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" && input.ChannelMappedModel != input.OriginalModel {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}
	billingModels := usageBillingModelCandidates(
		billingModel,
		result.BillingModel,
		input.ChannelMappedModel,
		input.OriginalModel,
		result.UpstreamModel,
		result.Model,
	)
	billingAccount := account
	if account != nil && account.IsShadow() && s.accountRepo != nil {
		if resolved, resolveErr := resolveCredentialAccount(ctx, s.accountRepo, account); resolveErr == nil && resolved != nil {
			billingAccount = resolved
		}
	}
	longContextBillingEnabled := billingAccount.IsOpenAILongContextBillingEnabled()
	accountRateMultiplier := account.BillingRateMultiplier()
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	if billingRequestID := strings.TrimSpace(result.BillingRequestID); billingRequestID != "" {
		requestID = "local:" + billingRequestID
	}
	if result.OpenAIWSMode && strings.TrimSpace(result.BillingRequestID) == "" && (s.usageSettlement == nil || !s.usageSettlement.Enabled()) {
		if upstreamRequestID := strings.TrimSpace(result.RequestID); upstreamRequestID != "" {
			requestID = upstreamRequestID
		}
	}

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}

	isVideoUsage := isGrokVideoUsageResult(result, billingModels)
	videoCount := 0
	videoResolution := result.VideoResolution
	videoDurationSeconds := result.VideoDurationSeconds
	if isVideoUsage {
		videoCount = result.VideoCount
		videoResolution = NormalizeVideoBillingResolutionOrDefault(result.VideoResolution)
		videoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(result.VideoDurationSeconds)
	}
	quotaPlatform := input.QuotaPlatform
	if quotaPlatform == "" {
		quotaPlatform = PlatformFromAPIKey(apiKey)
	}
	return settlePreparedUsage(ctx, appbilling.RecordInput{
		Policy: appbilling.RecordPolicy{
			Provider:                 appbilling.ProviderOpenAI,
			PricingMissing:           appbilling.PricingMissingLogZeroCost,
			CostErrors:               appbilling.CostErrorLogZeroPricingOnly,
			TokenBuckets:             appbilling.TokenBucketsOpenAIExclusiveInput,
			OpenAILongContextEnabled: longContextBillingEnabled,
		},
		RequestID:        requestID,
		BillingRequestID: result.BillingRequestID,
		Model:            result.Model,
		RequestedModel:   requestedModel,
		UpstreamModel:    result.UpstreamModel,
		// UpstreamResponseModel 是上游响应声明的模型（网关改写回客户端模型之前），
		// 落库后可在用量记录里看到真实路由落点，并标记上游与请求不一致的请求。
		UpstreamResponseModel: result.ResponseModel,
		BillingModels:         billingModels,
		QuotaPlatform:         quotaPlatform,
		Usage:                 tokens,
		InboundEndpoint:       input.InboundEndpoint,
		UpstreamEndpoint:      input.UpstreamEndpoint,
		UserAgent:             input.UserAgent,
		IPAddress:             input.IPAddress,
		RequestPayloadHash:    input.RequestPayloadHash,
		ChannelID:             input.ChannelID,
		ModelMappingChain:     input.ModelMappingChain,
		RateMultiplier:        multiplier,
		ImageMultiplier:       imageMultiplier,
		VideoMultiplier:       videoMultiplier,
		AccountRateMultiplier: accountRateMultiplier,
		BillingType:           BillingTypeBalance,
		ServiceTier:           result.ServiceTier,
		ReasoningEffort:       result.ReasoningEffort,
		Stream:                result.Stream,
		Duration:              result.Duration,
		FirstTokenMs:          result.FirstTokenMs,
		FirstOutputStages:     result.FirstOutputStages,
		ImageCount:            result.ImageCount,
		ImageSize:             result.ImageSize,
		ImageInputSize:        result.ImageInputSize,
		ImageOutputSize:       result.ImageOutputSize,
		ImageSizeSource:       result.ImageSizeSource,
		ImageSizeBreakdown:    result.ImageSizeBreakdown,
		VideoCount:            videoCount,
		VideoUsage:            isVideoUsage,
		VideoResolution:       videoResolution,
		VideoDurationSeconds:  videoDurationSeconds,
		Compact:               result.Compact,
		CompactPayloadBytes:   result.CompactPayloadBytes,
		CompactRetryCount:     result.CompactRetryCount,
		CompactClientCanceled: result.CompactClientCanceled,
		CyberBlocked:          input.CyberBlocked,
		OpenAIWSMode:          result.OpenAIWSMode,
		WSPayloadBytes:        result.WSPayloadBytes,
		WSEventCount:          result.WSEventCount,
		WSConnReused:          result.WSConnReused,
		WSConnPickMs:          result.WSConnPickMs,
		WSQueueWaitMs:         result.WSQueueWaitMs,
		Account:               account,
		APIKey:                apiKey,
		User:                  user,
		Subscription:          subscription,
	}, usageBillingExecutionDeps{
		Cost: usageBillingCostDeps{
			BillingService: s.billingService,
			Resolver:       s.resolver,
			RefreshAPIKey:  s.apiKeyWithFreshGroupMediaPricing,
		},
		Coordinator:   s.usageSettlement,
		Billing:       s.billingDeps(),
		BillingRepo:   s.usageBillingRepo,
		UsageLogRepo:  s.usageLogRepo,
		APIKeyService: input.APIKeyService,
		Component:     "service.openai_gateway",
		OnCostError: func(err error) {
			logger.L().With(
				zap.String("component", "service.openai_gateway"),
				zap.Strings("billing_models", billingModels),
				zap.String("requested_model", input.OriginalModel),
				zap.String("mapped_model", input.ChannelMappedModel),
				zap.String("upstream_model", result.UpstreamModel),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Warn("openai_usage.pricing_missing_record_zero_cost", zap.Error(err))
		},
	})
}

func isUsagePricingUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrModelPricingUnavailable) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no pricing available") || strings.Contains(msg, "pricing not found")
}

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &ImagePriceConfig{
		Price1K: apiKey.Group.ImagePrice1K,
		Price2K: apiKey.Group.ImagePrice2K,
		Price4K: apiKey.Group.ImagePrice4K,
	}
}

func apiKeyHasConfiguredImagePrice(apiKey *APIKey, sizeTier string) bool {
	cfg := imagePriceConfigFromAPIKey(apiKey)
	if cfg == nil {
		return false
	}
	switch NormalizeImageBillingTierOrDefault(sizeTier) {
	case ImageBillingSize1K:
		return cfg.Price1K != nil
	case ImageBillingSize2K:
		return cfg.Price2K != nil
	case ImageBillingSize4K:
		return cfg.Price4K != nil
	default:
		return false
	}
}

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &VideoPriceConfig{
		Price480P:  apiKey.Group.VideoPrice480P,
		Price720P:  apiKey.Group.VideoPrice720P,
		Price1080P: apiKey.Group.VideoPrice1080P,
	}
}

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, resolution string) bool {
	cfg := videoPriceConfigFromAPIKey(apiKey)
	if cfg == nil {
		return false
	}
	switch NormalizeVideoBillingResolutionOrDefault(resolution) {
	case VideoBillingResolution480P:
		return cfg.Price480P != nil
	case VideoBillingResolution720P:
		return cfg.Price720P != nil
	case VideoBillingResolution1080P:
		return cfg.Price1080P != nil
	default:
		return false
	}
}

func isGrokVideoUsageResult(result *OpenAIForwardResult, billingModels []string) bool {
	if result != nil && result.VideoCount > 0 {
		return true
	}
	for _, model := range billingModels {
		model = strings.TrimSpace(model)
		if strings.HasPrefix(model, "grok-imagine-video") {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) apiKeyWithFreshGroupMediaPricing(ctx context.Context, apiKey *APIKey) *APIKey {
	if apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 {
		return apiKey
	}
	if !groupMediaPricingLooksIncomplete(apiKey.Group) {
		return apiKey
	}
	if s == nil || s.channelService == nil || s.channelService.groupRepo == nil {
		return apiKey
	}
	group, err := s.channelService.groupRepo.GetByIDLite(ctx, *apiKey.GroupID)
	if err != nil || group == nil {
		return apiKey
	}
	clone := *apiKey
	clone.Group = group
	return &clone
}

// groupMediaPricingLooksIncomplete 判断分组对象是否可能缺失媒体计费字段（例如由不含
// 这些字段的旧快照或手工构造的上下文对象生成）。image/video 独立倍率在数据库中的
// 默认值均为 1.0，正常加载的分组不可能两个倍率同时为 0 且未开启独立倍率、全部媒体
// 价为 nil——只有这种情况才回源查库，避免对未配置覆盖价的分组每条媒体用量都多打一次 DB 查询。
func groupMediaPricingLooksIncomplete(group *Group) bool {
	if group == nil {
		return true
	}
	if group.ImageRateIndependent || group.VideoRateIndependent {
		return false
	}
	if group.ImageRateMultiplier != 0 || group.VideoRateMultiplier != 0 {
		return false
	}
	return group.ImagePrice1K == nil && group.ImagePrice2K == nil && group.ImagePrice4K == nil &&
		group.VideoPrice480P == nil && group.VideoPrice720P == nil && group.VideoPrice1080P == nil
}

// ParseCodexRateLimitHeaders extracts Codex usage limits from response headers.
// Exported for use in ratelimit_service when handling OpenAI 429 responses.
func ParseCodexRateLimitHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{}
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
		return nil
	}
	parseUsedPercent := func(key string) *float64 {
		return roundUsagePercent2Ptr(parseFloat(key))
	}

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
			}
		}
		return nil
	}

	// Primary (weekly) limits
	if v := parseUsedPercent("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
	}

	// Secondary (5h) limits
	if v := parseUsedPercent("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
	}

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
	}

	if !hasData {
		return nil
	}

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
}

func codexSnapshotBaseTime(snapshot *OpenAICodexUsageSnapshot, fallback time.Time) time.Time {
	if snapshot == nil {
		return fallback
	}
	if snapshot.UpdatedAt == "" {
		return fallback
	}
	base, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	if err != nil {
		return fallback
	}
	return base
}

func codexResetAtRFC3339(base time.Time, resetAfterSeconds *int) *string {
	if resetAfterSeconds == nil {
		return nil
	}
	sec := *resetAfterSeconds
	if sec < 0 {
		sec = 0
	}
	resetAt := base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339)
	return &resetAt
}

func buildCodexUsageExtraUpdates(snapshot *OpenAICodexUsageSnapshot, fallbackNow time.Time) map[string]any {
	if snapshot == nil {
		return nil
	}

	baseTime := codexSnapshotBaseTime(snapshot, fallbackNow)
	updates := map[string]any{
		// A fresh snapshot is authoritative. Persist explicit nulls for upstream
		// windows that disappeared so an account migrated to weekly-only limits
		// cannot keep stale primary/secondary values from an older 5h snapshot.
		"codex_primary_used_percent":           nil,
		"codex_primary_reset_after_seconds":    nil,
		"codex_primary_window_minutes":         nil,
		"codex_secondary_used_percent":         nil,
		"codex_secondary_reset_after_seconds":  nil,
		"codex_secondary_window_minutes":       nil,
		"codex_primary_over_secondary_percent": nil,
	}

	// 保存原始 primary/secondary 字段，便于排查问题
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = roundUsagePercent2(*snapshot.PrimaryUsedPercent)
	}
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	}
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
	}
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = roundUsagePercent2(*snapshot.SecondaryUsedPercent)
	}
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	}
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
	}
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
	}
	updates["codex_usage_updated_at"] = baseTime.Format(time.RFC3339)

	// 归一化到 5h/7d 规范字段
	if normalized := snapshot.Normalize(); normalized != nil {
		// Once upstream supplies window durations, their presence is authoritative.
		// Clear both canonical buckets first, then populate only windows that exist.
		// This removes a previously persisted 5h window after OpenAI migrates an
		// account to a weekly-only quota model.
		if snapshot.PrimaryWindowMinutes != nil || snapshot.SecondaryWindowMinutes != nil {
			updates["codex_5h_used_percent"] = nil
			updates["codex_5h_reset_after_seconds"] = nil
			updates["codex_5h_window_minutes"] = nil
			updates["codex_5h_reset_at"] = nil
			updates["codex_7d_used_percent"] = nil
			updates["codex_7d_reset_after_seconds"] = nil
			updates["codex_7d_window_minutes"] = nil
			updates["codex_7d_reset_at"] = nil
		}
		if normalized.Used5hPercent != nil {
			updates["codex_5h_used_percent"] = roundUsagePercent2(*normalized.Used5hPercent)
		}
		if normalized.Reset5hSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *normalized.Reset5hSeconds
		}
		if normalized.Window5hMinutes != nil {
			updates["codex_5h_window_minutes"] = *normalized.Window5hMinutes
		}
		if normalized.Used7dPercent != nil {
			updates["codex_7d_used_percent"] = roundUsagePercent2(*normalized.Used7dPercent)
		}
		if normalized.Reset7dSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *normalized.Reset7dSeconds
		}
		if normalized.Window7dMinutes != nil {
			updates["codex_7d_window_minutes"] = *normalized.Window7dMinutes
		}
		if reset5hAt := codexResetAtRFC3339(baseTime, normalized.Reset5hSeconds); reset5hAt != nil {
			updates["codex_5h_reset_at"] = *reset5hAt
		}
		if reset7dAt := codexResetAtRFC3339(baseTime, normalized.Reset7dSeconds); reset7dAt != nil {
			updates["codex_7d_reset_at"] = *reset7dAt
		}
	}

	return updates
}

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
// updateCodexUsageSnapshot 把 /responses 的 x-codex-* 全局头快照写入账号 codex_* Extra。
// ⚠️ 调用方必须排除 spark 影子账号(account.IsShadow()):影子的 codex_* 仅由 QueryUsage
// (/wham/usage bengalfox 道)更新,不能被全局头口径污染(外审第7轮 P1)。本函数仅持 accountID,
// 无法在此自检影子,故守卫前置到各调用点。
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
	}
	if s == nil || s.accountRepo == nil {
		return
	}

	now := time.Now()
	updates := buildCodexUsageExtraUpdates(snapshot, now)
	if len(updates) == 0 {
		return
	}
	if !s.getCodexSnapshotThrottle().Allow(accountID, now) {
		return
	}

	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
		s.observeCodexQuotaWindows(updateCtx, accountID, updates, now)
	}()
}

func (s *OpenAIGatewayService) observeCodexQuotaWindows(ctx context.Context, accountID int64, updates map[string]any, now time.Time) {
	if s == nil {
		return
	}
	observeCodexQuotaWindowUpdates(ctx, s.quotaWindowLedger, accountID, updates, now)
}

func (s *OpenAIGatewayService) UpdateCodexUsageSnapshotFromHeaders(ctx context.Context, accountID int64, headers http.Header) {
	if accountID <= 0 || headers == nil {
		return
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, accountID, snapshot)
	}
}
