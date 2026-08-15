package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appbilling "github.com/Wei-Shaw/sub2api/internal/app/billing"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

type usageBillingPlatform uint8

const (
	usageBillingPlatformGateway usageBillingPlatform = iota
	usageBillingPlatformOpenAI
)

type usageBillingCostRequest struct {
	Platform                   usageBillingPlatform
	APIKey                     *APIKey
	BillingModels              []string
	Tokens                     UsageTokens
	ImageCount                 int
	ImageSize                  string
	VideoCount                 int
	VideoResolution            string
	VideoDurationSeconds       int
	RateMultiplier             float64
	ImageMultiplier            float64
	VideoMultiplier            float64
	ServiceTier                string
	FixedLongContextThreshold  int
	FixedLongContextMultiplier float64
	OpenAILongContextEnabled   bool
}

type usageBillingCostDeps struct {
	BillingService *BillingService
	Resolver       *ModelPricingResolver
	RefreshAPIKey  func(context.Context, *APIKey) *APIKey
}

type usageBillingExecutionDeps struct {
	Cost          usageBillingCostDeps
	Coordinator   ports.UsageSettlementCoordinator
	Billing       *billingDeps
	BillingRepo   UsageBillingRepository
	UsageLogRepo  UsageLogRepository
	APIKeyService APIKeyQuotaUpdater
	Component     string
	OnCostError   func(error)
}

func persistUsageBilling(
	ctx context.Context,
	coordinator ports.UsageSettlementCoordinator,
	usageLog *UsageLog,
	params *postUsageBillingParams,
	deps *billingDeps,
	billingRepo UsageBillingRepository,
	usageLogRepo UsageLogRepository,
	component string,
) error {
	if authoritative, err := handoffUsageSettlement(ctx, coordinator, usageLog, params, params.Platform); err != nil {
		return err
	} else if authoritative {
		return nil
	}
	if _, err := applyUsageBilling(ctx, usageLog.RequestID, usageLog, params, deps, billingRepo); err != nil {
		return err
	}
	writeUsageLogBestEffort(ctx, usageLogRepo, usageLog, component)
	return nil
}

func settlePreparedUsage(ctx context.Context, input appbilling.RecordInput, deps usageBillingExecutionDeps) error {
	if err := validateUsageBillingPolicy(input.Policy); err != nil {
		return err
	}
	platform := usageBillingPlatformGateway
	if input.Policy.Provider == appbilling.ProviderOpenAI {
		platform = usageBillingPlatformOpenAI
	}
	cost, err := calculateUsageBillingCost(ctx, deps.Cost, usageBillingCostRequest{
		Platform:                   platform,
		APIKey:                     input.APIKey,
		BillingModels:              input.BillingModels,
		Tokens:                     input.Usage,
		ImageCount:                 input.ImageCount,
		ImageSize:                  input.ImageSize,
		VideoCount:                 input.VideoCount,
		VideoResolution:            input.VideoResolution,
		VideoDurationSeconds:       input.VideoDurationSeconds,
		RateMultiplier:             input.RateMultiplier,
		ImageMultiplier:            input.ImageMultiplier,
		VideoMultiplier:            input.VideoMultiplier,
		ServiceTier:                usageBillingStringValue(input.ServiceTier),
		FixedLongContextThreshold:  input.Policy.FixedLongContextThreshold,
		FixedLongContextMultiplier: input.Policy.FixedLongContextMultiplier,
		OpenAILongContextEnabled:   input.Policy.OpenAILongContextEnabled,
	})
	if err != nil {
		pricingMissingZero := input.Policy.PricingMissing == appbilling.PricingMissingLogZeroCost && isUsagePricingUnavailableError(err)
		canRecordZero := input.Policy.CostErrors == appbilling.CostErrorLogZeroAll ||
			(input.Policy.CostErrors == appbilling.CostErrorLogZeroPricingOnly && pricingMissingZero)
		if !canRecordZero {
			return err
		}
		if deps.OnCostError != nil {
			deps.OnCostError(err)
		}
		cost = &CostBreakdown{ActualCost: 0}
		if input.Policy.Provider == appbilling.ProviderOpenAI {
			cost.BillingMode = string(BillingModeToken)
		}
	}

	effectiveRateMultiplier := input.RateMultiplier
	if input.VideoUsage && (cost == nil || cost.BillingMode != string(BillingModeToken)) {
		effectiveRateMultiplier = input.VideoMultiplier
	} else if input.ImageCount > 0 && (cost == nil || cost.BillingMode != string(BillingModeToken)) {
		effectiveRateMultiplier = input.ImageMultiplier
	}
	usageLog := appbilling.BuildUsageLog(input, cost, effectiveRateMultiplier)
	params := &postUsageBillingParams{
		Cost:                  cost,
		User:                  input.User,
		APIKey:                input.APIKey,
		Account:               input.Account,
		Subscription:          input.Subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    false,
		AccountRateMultiplier: input.AccountRateMultiplier,
		APIKeyService:         deps.APIKeyService,
		Platform:              input.QuotaPlatform,
	}
	return persistUsageBilling(ctx, deps.Coordinator, usageLog, params, deps.Billing, deps.BillingRepo, deps.UsageLogRepo, deps.Component)
}

func validateUsageBillingPolicy(policy appbilling.RecordPolicy) error {
	switch policy.Provider {
	case appbilling.ProviderGateway:
		if policy.TokenBuckets != appbilling.TokenBucketsGateway ||
			policy.CostErrors != appbilling.CostErrorLogZeroAll ||
			policy.PricingMissing != appbilling.PricingMissingLogZeroCost {
			return errors.New("invalid gateway usage billing policy")
		}
	case appbilling.ProviderOpenAI:
		if policy.TokenBuckets != appbilling.TokenBucketsOpenAIExclusiveInput ||
			policy.CostErrors != appbilling.CostErrorLogZeroPricingOnly ||
			policy.PricingMissing != appbilling.PricingMissingLogZeroCost {
			return errors.New("invalid OpenAI usage billing policy")
		}
	default:
		return fmt.Errorf("unknown usage billing provider %q", policy.Provider)
	}
	return nil
}

func usageBillingStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func calculateUsageBillingCost(ctx context.Context, deps usageBillingCostDeps, request usageBillingCostRequest) (*CostBreakdown, error) {
	billingModel := firstUsageBillingModel(request.BillingModels)
	isVideo := request.VideoCount > 0
	if request.Platform == usageBillingPlatformOpenAI && !isVideo {
		for _, candidate := range request.BillingModels {
			if strings.HasPrefix(strings.TrimSpace(candidate), "grok-imagine-video") {
				isVideo = true
				break
			}
		}
	}
	if isVideo {
		if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, request.APIKey); resolved == nil || resolved.Mode != BillingModeToken {
			return calculateUsageVideoCost(ctx, deps, request, billingModel), nil
		}
	}
	if request.ImageCount > 0 {
		if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, request.APIKey); resolved == nil || resolved.Mode != BillingModeToken {
			return calculateUsageImageCost(ctx, deps, request, billingModel), nil
		}
	}
	if len(request.BillingModels) == 0 || billingModel == "" {
		return nil, errors.New("usage billing model is empty")
	}
	var lastErr error
	for _, candidate := range request.BillingModels {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		cost, err := calculateUsageTokenCost(ctx, deps, request, candidate)
		if err == nil {
			return cost, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no non-empty billing model candidates")
	}
	return nil, fmt.Errorf("calculate usage cost failed for billing models %s: %w", strings.Join(request.BillingModels, ","), lastErr)
}

func calculateUsageTokenCost(ctx context.Context, deps usageBillingCostDeps, request usageBillingCostRequest, billingModel string) (*CostBreakdown, error) {
	if request.Platform == usageBillingPlatformGateway {
		if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, request.APIKey); resolved != nil {
			groupID := request.APIKey.Group.ID
			return deps.BillingService.CalculateCostUnified(CostInput{
				Ctx: ctx, Model: billingModel, GroupID: &groupID, Tokens: request.Tokens,
				RequestCount: 1, RateMultiplier: request.RateMultiplier,
				Resolver: deps.Resolver, Resolved: resolved,
			})
		}
		if request.FixedLongContextThreshold > 0 {
			return deps.BillingService.CalculateCostWithLongContext(
				billingModel, request.Tokens, request.RateMultiplier,
				request.FixedLongContextThreshold, request.FixedLongContextMultiplier,
			)
		}
		return deps.BillingService.CalculateCost(billingModel, request.Tokens, request.RateMultiplier)
	}
	if deps.Resolver != nil && request.APIKey != nil && request.APIKey.Group != nil {
		groupID := request.APIKey.Group.ID
		return deps.BillingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &groupID, Tokens: request.Tokens,
			RequestCount: 1, RateMultiplier: request.RateMultiplier, ServiceTier: request.ServiceTier,
			Resolver: deps.Resolver, LongContextBillingEnabled: &request.OpenAILongContextEnabled,
		})
	}
	return deps.BillingService.calculateCostWithServiceTierPolicy(
		billingModel, request.Tokens, request.RateMultiplier, request.ServiceTier, request.OpenAILongContextEnabled,
	)
}

func calculateUsageImageCost(ctx context.Context, deps usageBillingCostDeps, request usageBillingCostRequest, billingModel string) *CostBreakdown {
	sizeTier := NormalizeImageBillingTierOrDefault(request.ImageSize)
	if request.Platform == usageBillingPlatformGateway {
		if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, request.APIKey); resolved != nil {
			groupID := request.APIKey.Group.ID
			mediaTokens := UsageTokens{
				InputTokens:       request.Tokens.InputTokens,
				OutputTokens:      request.Tokens.OutputTokens,
				ImageOutputTokens: request.Tokens.ImageOutputTokens,
			}
			cost, err := deps.BillingService.CalculateCostUnified(CostInput{
				Ctx: ctx, Model: billingModel, GroupID: &groupID, Tokens: mediaTokens,
				RequestCount: request.ImageCount, SizeTier: sizeTier, RateMultiplier: request.ImageMultiplier,
				Resolver: deps.Resolver, Resolved: resolved,
			})
			if err != nil {
				logger.LegacyPrintf("service.gateway", "Calculate image token cost failed: %v", err)
				return &CostBreakdown{ActualCost: 0}
			}
			return cost
		}
		return deps.BillingService.CalculateImageCost(
			billingModel, sizeTier, request.ImageCount, imagePriceConfigFromAPIKey(request.APIKey), request.ImageMultiplier,
		)
	}

	apiKey := request.APIKey
	groupConfig := imagePriceConfigFromAPIKey(apiKey)
	if apiKeyHasConfiguredImagePrice(apiKey, sizeTier) {
		return deps.BillingService.CalculateImageCost(billingModel, sizeTier, request.ImageCount, groupConfig, request.ImageMultiplier)
	}
	if deps.RefreshAPIKey != nil {
		if refreshed := deps.RefreshAPIKey(ctx, apiKey); refreshed != apiKey {
			apiKey = refreshed
			groupConfig = imagePriceConfigFromAPIKey(apiKey)
			if apiKeyHasConfiguredImagePrice(apiKey, sizeTier) {
				return deps.BillingService.CalculateImageCost(billingModel, sizeTier, request.ImageCount, groupConfig, request.ImageMultiplier)
			}
		}
	}
	if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, apiKey); resolved != nil &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		groupID := apiKey.Group.ID
		cost, err := deps.BillingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &groupID, RequestCount: request.ImageCount,
			SizeTier: sizeTier, RateMultiplier: request.ImageMultiplier, Resolver: deps.Resolver, Resolved: resolved,
		})
		if err == nil {
			return cost
		}
		logger.LegacyPrintf("service.openai_gateway", "Calculate image channel cost failed: %v", err)
	}
	return deps.BillingService.CalculateImageCost(billingModel, sizeTier, request.ImageCount, groupConfig, request.ImageMultiplier)
}

func calculateUsageVideoCost(ctx context.Context, deps usageBillingCostDeps, request usageBillingCostRequest, billingModel string) *CostBreakdown {
	videoCount := request.VideoCount
	if videoCount <= 0 {
		videoCount = 1
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(request.VideoResolution)
	durationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(request.VideoDurationSeconds)
	apiKey := request.APIKey
	groupConfig := videoPriceConfigFromAPIKey(apiKey)
	if apiKeyHasConfiguredVideoPrice(apiKey, resolution) {
		return deps.BillingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, request.VideoMultiplier)
	}
	if deps.RefreshAPIKey != nil {
		if refreshed := deps.RefreshAPIKey(ctx, apiKey); refreshed != apiKey {
			apiKey = refreshed
			groupConfig = videoPriceConfigFromAPIKey(apiKey)
			if apiKeyHasConfiguredVideoPrice(apiKey, resolution) {
				return deps.BillingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, request.VideoMultiplier)
			}
		}
	}
	if resolved := resolveUsageChannelPricing(ctx, deps.Resolver, billingModel, apiKey); resolved != nil &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		groupID := apiKey.Group.ID
		cost, err := deps.BillingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &groupID, RequestCount: videoCount,
			SizeTier: resolution, RateMultiplier: request.VideoMultiplier, Resolver: deps.Resolver, Resolved: resolved,
		})
		if err == nil {
			cost.BillingMode = string(BillingModeVideo)
			return cost
		}
		logger.LegacyPrintf("service.openai_gateway", "Calculate video channel cost failed: %v", err)
	}
	return deps.BillingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, request.VideoMultiplier)
}

func resolveUsageChannelPricing(ctx context.Context, resolver *ModelPricingResolver, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if resolver == nil || apiKey == nil || apiKey.Group == nil {
		return nil
	}
	groupID := apiKey.Group.ID
	resolved := resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &groupID})
	if resolved.Source != PricingSourceChannel {
		return nil
	}
	return resolved
}

func resolveUsageRateMultiplier(
	ctx context.Context,
	cfg *config.Config,
	resolver *userGroupRateResolver,
	userID int64,
	apiKey *APIKey,
	fallback func() *userGroupRateResolver,
) float64 {
	multiplier := 1.0
	if cfg != nil {
		multiplier = cfg.Default.RateMultiplier
	}
	if apiKey == nil || apiKey.GroupID == nil || apiKey.Group == nil {
		return multiplier
	}
	if resolver == nil && fallback != nil {
		resolver = fallback()
	}
	if resolver == nil {
		return apiKey.Group.RateMultiplier
	}
	return resolver.Resolve(ctx, userID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
}
