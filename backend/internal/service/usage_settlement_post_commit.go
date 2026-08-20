package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UsageSettlementPostCommit struct {
	userRepo               UserRepository
	accountRepo            AccountRepository
	apiKeyRepo             APIKeyRepository
	billingCache           *BillingCacheService
	deferred               *DeferredService
	balanceNotify          *BalanceNotifyService
	apiKeyCacheInvalidator APIKeyAuthCacheInvalidator
	userPlatformQuotaRepo  UserPlatformQuotaRepository
	cfg                    *config.Config
}

func NewUsageSettlementPostCommit(
	userRepo UserRepository,
	accountRepo AccountRepository,
	apiKeyRepo APIKeyRepository,
	billingCache *BillingCacheService,
	deferred *DeferredService,
	balanceNotify *BalanceNotifyService,
	apiKeyCacheInvalidator APIKeyAuthCacheInvalidator,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
	cfg *config.Config,
) *UsageSettlementPostCommit {
	return &UsageSettlementPostCommit{
		userRepo: userRepo, accountRepo: accountRepo, apiKeyRepo: apiKeyRepo,
		billingCache: billingCache, deferred: deferred, balanceNotify: balanceNotify,
		apiKeyCacheInvalidator: apiKeyCacheInvalidator, userPlatformQuotaRepo: userPlatformQuotaRepo, cfg: cfg,
	}
}

func (s *UsageSettlementPostCommit) AfterUsageSettlement(ctx context.Context, attempt *domain.UsageSettlementAttempt, result *domain.UsageSettlementApplyResult) {
	if s == nil || attempt == nil || attempt.Payload == nil || result == nil || result.AlreadySettled || !result.Billing.Applied {
		return
	}
	payload := attempt.Payload
	billing := payload.Billing
	usageLog := payload.UsageLog

	if s.billingCache != nil {
		if billing.SubscriptionCost > 0 && usageLog.GroupID != nil {
			s.billingCache.QueueUpdateSubscriptionUsage(billing.UserID, *usageLog.GroupID, billing.SubscriptionCost)
		} else if billing.BalanceCost > 0 {
			if result.Billing.NewBalance != nil && s.billingCache.balanceBelowEligibilityThreshold(*result.Billing.NewBalance) {
				if err := s.billingCache.InvalidateUserBalance(ctx, billing.UserID); err != nil {
					slog.Warn("invalidate balance cache after durable settlement failed", "user_id", billing.UserID, "error", err)
				}
			} else {
				s.billingCache.QueueDeductBalance(billing.UserID, billing.BalanceCost)
			}
		}
		if billing.APIKeyRateLimitCost > 0 {
			s.billingCache.QueueUpdateAPIKeyRateLimitUsage(billing.APIKeyID, billing.APIKeyRateLimitCost)
		}
	}

	if s.deferred != nil {
		s.deferred.ScheduleLastUsedUpdate(billing.AccountID)
	}
	if result.Billing.APIKeyQuotaExhausted && s.apiKeyRepo != nil && s.apiKeyCacheInvalidator != nil {
		if apiKey, err := s.apiKeyRepo.GetByID(ctx, billing.APIKeyID); err != nil {
			slog.Warn("load api key after durable settlement exhaustion failed", "api_key_id", billing.APIKeyID, "error", err)
		} else if apiKey != nil && apiKey.Key != "" {
			s.apiKeyCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
		}
	}

	s.updatePlatformQuotaMirror(ctx, payload)
	s.notifySettlement(ctx, payload, &result.Billing)
}

func (s *UsageSettlementPostCommit) updatePlatformQuotaMirror(ctx context.Context, payload *domain.UsageSettlementPayloadV1) {
	if s == nil || payload == nil || s.billingCache == nil || s.userPlatformQuotaRepo == nil ||
		payload.QuotaPlatform == "" || payload.Billing.BalanceCost <= 0 {
		return
	}
	userID := payload.Billing.UserID
	platform := payload.QuotaPlatform
	cost := payload.Billing.BalanceCost
	if !s.billingCache.HasUserPlatformQuotaLimit(ctx, userID, platform) {
		return
	}
	s.billingCache.IncrementUserPlatformQuotaUsage(userID, platform, cost)
	if s.cfg != nil && s.cfg.Database.UserPlatformQuotaFlusherEnabled {
		return
	}
	// Billing already committed. Do not bind the DB mirror write to the
	// worker lease ctx — a just-expired lease would drop this increment.
	dbCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	if err := s.userPlatformQuotaRepo.IncrementUsageWithReset(dbCtx, userID, platform, cost, time.Now().UTC()); err != nil {
		userPlatformQuotaDBIncrErrorTotal.Add(1)
		slog.Error("persist platform quota mirror after durable settlement failed", "user_id", userID, "platform", platform, "cost", cost, "error", err)
	}
}

func (s *UsageSettlementPostCommit) notifySettlement(ctx context.Context, payload *domain.UsageSettlementPayloadV1, result *UsageBillingApplyResult) {
	if s == nil || payload == nil || s.balanceNotify == nil || s.userRepo == nil || s.accountRepo == nil {
		return
	}
	user, userErr := s.userRepo.GetByID(ctx, payload.Billing.UserID)
	account, accountErr := s.accountRepo.GetByID(ctx, payload.Billing.AccountID)
	if userErr != nil || accountErr != nil || user == nil || account == nil {
		slog.Warn("load settlement subjects for notifications failed", "user_id", payload.Billing.UserID, "account_id", payload.Billing.AccountID, "user_error", userErr, "account_error", accountErr)
		return
	}
	accountRate := 1.0
	if payload.UsageLog.AccountRateMultiplier != nil {
		accountRate = *payload.UsageLog.AccountRateMultiplier
	}
	params := &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: payload.UsageLog.ActualCost, TotalCost: payload.UsageLog.TotalCost},
		User: user, Account: account, AccountRateMultiplier: accountRate,
		IsSubscriptionBill: payload.Billing.SubscriptionCost > 0,
	}
	notifyBalanceLow(params, &billingDeps{balanceNotifyService: s.balanceNotify}, result)
	notifyAccountQuota(params, &billingDeps{balanceNotifyService: s.balanceNotify}, result)
}
