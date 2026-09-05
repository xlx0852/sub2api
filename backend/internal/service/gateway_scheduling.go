package service

// 本文件由 gateway_service.go 纯移动拆分而来：账号选择与负载感知调度、窗口费用
// 与 RPM 预取、候选排序/过滤、混合平台调度与选择失败诊断。仅做代码搬迁，
// 无任何行为变更。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

var errGatewayLoadSnapshotUnavailable = errors.New("gateway load snapshot unavailable")

type gatewayPlatformHintContextKey struct{}

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	selection, err := AccountSelectionFromScheduling(NewGatewayAccountSelector(s).Select(ctx, scheduling.Request{
		Platform:       PlatformAnthropic,
		GroupID:        groupID,
		StickyKey:      sessionHash,
		RequestedModel: requestedModel,
		ExcludedIDs:    excludedIDs,
		SkipAcquire:    true,
	}))
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil {
		return nil, ErrNoAvailableAccounts
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return selection.Account, nil
}

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
// metadataUserID: 用于客户端亲和调度，从中提取客户端 ID
// sub2apiUserID: 系统用户 ID，用于二维亲和调度
func (s *GatewayService) selectAccountWithLoadAwarenessStrategy(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	// 调试日志：记录调度入口参数
	excludedIDsList := make([]int64, 0, len(excludedIDs))
	for id := range excludedIDs {
		excludedIDsList = append(excludedIDsList, id)
	}
	slog.Debug("account_scheduling_starting",
		"group_id", derefGroupID(groupID),
		"model", requestedModel,
		"session", shortSessionHash(sessionHash),
		"excluded_ids", excludedIDsList)

	cfg := s.schedulingConfig()

	// 检查 Claude Code 客户端限制（可能会替换 groupID 为降级分组）
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ctx = s.withGroupContext(ctx, group)

	// Claude Code 限制可能已将 groupID 解析为 fallback group，
	// 渠道限制预检查必须使用解析后的分组。
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	var stickyAccountID int64
	var stickySource string
	if prefetch := prefetchedStickyAccountIDFromContext(ctx, groupID); prefetch > 0 {
		stickyAccountID = prefetch
		stickySource = "prefetch"
	} else if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); err == nil {
			stickyAccountID = accountID
			stickySource = "cache"
		}
	}

	// [DEBUG-STICKY] 调度器入口日志
	slog.Info("sticky.scheduler_entry",
		"group_id", derefGroupID(groupID),
		"session_hash", shortSessionHash(sessionHash),
		"sticky_account_id", stickyAccountID,
		"sticky_source", stickySource,
		"model", requestedModel,
		"load_batch", cfg.LoadBatchEnabled,
		"has_concurrency_svc", s.concurrencyService != nil,
		"excluded_count", len(excludedIDs),
	)

	if s.debugModelRoutingEnabled() && requestedModel != "" {
		groupPlatform := ""
		if group != nil {
			groupPlatform = group.Platform
		}
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] select entry: group_id=%v group_platform=%s model=%s session=%s sticky_account=%d load_batch=%v concurrency=%v",
			derefGroupID(groupID), groupPlatform, requestedModel, shortSessionHash(sessionHash), stickyAccountID, cfg.LoadBatchEnabled, s.concurrencyService != nil)
	}

	platform, hasForcePlatform, err := s.resolvePlatform(ctx, groupID, group)
	if err != nil {
		return nil, err
	}
	preferOAuth := platform == PlatformGemini
	if s.debugModelRoutingEnabled() && platform == PlatformAnthropic && requestedModel != "" {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] load-aware enabled: group_id=%v model=%s session=%s platform=%s", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), platform)
	}

	accounts, useMixed, err := s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 提前构建 accountByID（供 Layer 1 和 Layer 1.5 使用）
	accountByID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		accountByID[accounts[i].ID] = &accounts[i]
	}
	isExcluded := func(accountID int64) bool {
		return scheduling.IsExcluded(excludedIDs, accountID)
	}

	// 获取模型路由配置（仅 anthropic 平台）
	var routingAccountIDs []int64
	if group != nil && requestedModel != "" && group.Platform == PlatformAnthropic {
		routingAccountIDs = group.GetRoutingAccountIDs(requestedModel)
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v session=%s sticky_account=%d",
				group.ID, requestedModel, group.ModelRoutingEnabled, len(group.ModelRouting), routingAccountIDs, shortSessionHash(sessionHash), stickyAccountID)
			if len(routingAccountIDs) == 0 && group.ModelRoutingEnabled && len(group.ModelRouting) > 0 {
				keys := make([]string, 0, len(group.ModelRouting))
				for k := range group.ModelRouting {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				const maxKeys = 20
				if len(keys) > maxKeys {
					keys = keys[:maxKeys]
				}
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing miss: group_id=%d model=%s patterns(sample)=%v", group.ID, requestedModel, keys)
			}
		}
	}
	rolloutMode := s.schedulerScoringRolloutMode(ctx, platform, groupID)
	if rolloutMode == scheduling.RolloutShadow {
		shadowRequest := scheduling.Request{
			Platform:           platform,
			RequestedModel:     requestedModel,
			ExcludedIDs:        excludedIDs,
			AllowMixedPlatform: useMixed,
		}
		shadowPolicyDecision := s.evaluateGatewayCandidates(ctx, candidatesFromAccountValues(accounts), shadowRequest)
		if len(routingAccountIDs) > 0 {
			shadowPolicyDecision = s.evaluateGatewayRoutingShadowCandidates(ctx, accounts, shadowRequest, routingAccountIDs)
		}
		s.evaluateGatewayScoringShadow(
			ctx,
			platform,
			groupID,
			sessionHash,
			requestedModel,
			stickyAccountID,
			preferOAuth,
			cfg.PreferSoonestReset,
			accounts,
			shadowPolicyDecision,
			nil,
			false,
		)
	}

	// ============ Layer 1: 模型路由优先选择（优先级高于粘性会话） ============
	if len(routingAccountIDs) > 0 {
		// 1. 过滤出路由列表中可调度的账号
		var routingCandidates []*Account
		var filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost int
		var modelScopeSkippedIDs []int64 // 记录因模型限流被跳过的账号 ID
		routingPolicyRequest := scheduling.Request{
			Platform:           platform,
			RequestedModel:     requestedModel,
			ExcludedIDs:        excludedIDs,
			AllowMixedPlatform: useMixed,
		}
		routingPolicies := s.gatewayCandidatePolicies()
		for _, routingAccountID := range routingAccountIDs {
			if isExcluded(routingAccountID) {
				filteredExcluded++
				continue
			}
			account, ok := accountByID[routingAccountID]
			if !ok {
				filteredMissing++
				continue
			}
			decision := scheduling.EvaluateCandidate(ctx, account, routingPolicyRequest, routingPolicies...)
			switch decision.RejectedPolicy {
			case "schedulable":
				filteredUnsched++
			case "gateway_platform":
				filteredPlatform++
			case "model_capability":
				filteredModelMapping++
			case "schedulable_for_model":
				filteredModelScope++
				modelScopeSkippedIDs = append(modelScopeSkippedIDs, account.ID)
			case "window_cost":
				filteredWindowCost++
			}
			if !decision.Allowed {
				continue
			}
			routingCandidates = append(routingCandidates, account)
		}

		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed candidates: group_id=%v model=%s routed=%d candidates=%d filtered(excluded=%d missing=%d unsched=%d platform=%d model_scope=%d model_mapping=%d window_cost=%d)",
				derefGroupID(groupID), requestedModel, len(routingAccountIDs), len(routingCandidates),
				filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost)
			if len(modelScopeSkippedIDs) > 0 {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] model_rate_limited accounts skipped: group_id=%v model=%s account_ids=%v",
					derefGroupID(groupID), requestedModel, modelScopeSkippedIDs)
			}
		}

		if len(routingCandidates) > 0 {
			// 1.5. 在路由账号范围内检查粘性会话
			if sessionHash != "" && stickyAccountID > 0 {
				slog.Debug("sticky.layer1_5_checking",
					"sticky_account_id", stickyAccountID,
					"in_routing_list", containsInt64(routingAccountIDs, stickyAccountID),
					"is_excluded", isExcluded(stickyAccountID),
					"in_account_map", func() bool { _, ok := accountByID[stickyAccountID]; return ok }(),
					"session", shortSessionHash(sessionHash),
				)
				if containsInt64(routingAccountIDs, stickyAccountID) && !isExcluded(stickyAccountID) {
					// 粘性账号在路由列表中，优先使用
					if stickyAccount, ok := accountByID[stickyAccountID]; ok {
						var stickyCacheMissReason string

						stickyPolicyRequest := routingPolicyRequest
						stickyPolicyRequest.Sticky = true
						stickyPolicyDecision := scheduling.EvaluateCandidate(ctx, stickyAccount, stickyPolicyRequest, routingPolicies...)
						rpmPass := stickyPolicyDecision.Allowed

						if rpmPass { // 粘性会话窗口费用+RPM 检查
							result, err := s.tryAcquireAccountSlot(ctx, requestedModel, stickyAccountID, stickyAccount.Concurrency)
							if err == nil && result.Acquired {
								// 会话数量限制检查
								if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
									result.ReleaseFunc() // 释放槽位
									stickyCacheMissReason = "session_limit"
									// 继续到负载感知选择
								} else {
									slog.Debug("sticky.layer1_5_hit",
										"account_id", stickyAccountID,
										"session", shortSessionHash(sessionHash),
										"result", "slot_acquired",
									)
									if s.debugModelRoutingEnabled() {
										logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), stickyAccountID)
									}
									return s.newAcquiredSelectionResult(ctx, stickyAccount, result.ReleaseFunc)
								}
							}
							if err == nil && result.ModelLimited {
								// 模型级并发预算已满：立即让路，不进入账号等待队列。
								return s.newModelLimitedSelectionResult(ctx, stickyAccount)
							}

							if stickyCacheMissReason == "" {
								waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, stickyAccountID)
								if waitingCount < cfg.StickySessionMaxWaiting {
									// 会话数量限制检查（等待计划也需要占用会话配额）
									if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
										stickyCacheMissReason = "session_limit"
										// 会话限制已满，继续到负载感知选择
									} else {
										// 必须走 newSelectionResult 以 hydrate 账号凭证：
										// 调度快照中的账号是精简版（OAuth token 等被剥离），
										// 直接返回会导致后续转发缺少凭证而鉴权失败。
										return s.newSelectionResult(ctx, stickyAccount, false, nil, newStickyWaitPlan(stickyAccountID, stickyAccount.Concurrency, cfg))
									}
								} else {
									stickyCacheMissReason = "wait_queue_full"
								}
							}
							// 粘性账号槽位满且等待队列已满，继续使用负载感知选择
						} else if stickyPolicyDecision.RejectedPolicy == "rpm" {
							stickyCacheMissReason = "rpm_red"
						} else {
							stickyCacheMissReason = "gate_check"
						}

						// 记录粘性缓存未命中的结构化日志
						if stickyCacheMissReason != "" {
							baseRPM := stickyAccount.GetBaseRPM()
							var currentRPM int
							if count, ok := rpmFromPrefetchContext(ctx, stickyAccount.ID); ok {
								currentRPM = count
							}
							logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=%s account_id=%d session=%s current_rpm=%d base_rpm=%d",
								stickyCacheMissReason, stickyAccountID, shortSessionHash(sessionHash), currentRPM, baseRPM)
						}
					} else {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=account_cleared account_id=%d session=%s current_rpm=0 base_rpm=0",
							stickyAccountID, shortSessionHash(sessionHash))
					}
				}
			}

			// 2. 批量获取负载信息
			routingLoads := make([]AccountWithConcurrency, 0, len(routingCandidates))
			for _, acc := range routingCandidates {
				routingLoads = append(routingLoads, AccountWithConcurrency{
					ID:             acc.ID,
					MaxConcurrency: acc.EffectiveLoadFactor(),
				})
			}
			var routingLoadMap map[int64]*AccountLoadInfo
			var routingLoadErr error
			if s.concurrencyService != nil && cfg.LoadBatchEnabled {
				routingLoadMap, routingLoadErr = s.concurrencyService.GetAccountsLoadBatch(ctx, routingLoads)
			} else {
				routingLoadErr = errGatewayLoadSnapshotUnavailable
			}
			s.reportGatewayCandidateSnapshots(ctx, routingCandidates, routingLoadMap)

			// 3. 由共享兼容排序器生成无副作用 acquire 顺序。
			routingAvailable := gatewayCompatibilitySelectionOrder(routingCandidates, routingLoadMap, routingLoadErr == nil, preferOAuth, false)

			if len(routingAvailable) > 0 {
				if rolloutMode == scheduling.RolloutEnforce && routingLoadErr == nil {
					routingPolicyDecision := s.evaluateGatewayRoutingShadowCandidates(ctx, accounts, routingPolicyRequest, routingAccountIDs)
					sharedOrder := s.gatewayEnforceSelectionOrder(
						ctx,
						platform,
						groupID,
						sessionHash,
						requestedModel,
						stickyAccountID,
						preferOAuth,
						cfg.PreferSoonestReset,
						accounts,
						routingPolicyDecision,
						routingLoadMap,
					)
					routingAvailable = orderGatewayAvailableByIDs(routingAvailable, sharedOrder)
				}

				// 4. 尝试获取槽位
				for _, item := range routingAvailable {
					result, err := s.tryAcquireAccountSlot(ctx, requestedModel, item.account.ID, item.account.Concurrency)
					if err == nil && result.Acquired {
						// 会话数量限制检查
						if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
							result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
							continue
						}
						if sessionHash != "" && s.cache != nil {
							_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, item.account.ID, stickySessionTTL)
						}
						if s.debugModelRoutingEnabled() {
							logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
						}
						return s.newAcquiredSelectionResult(ctx, item.account, result.ReleaseFunc)
					}
					if err == nil && result.ModelLimited {
						// 模型级并发预算已满：立即让路，不进入账号等待队列。
						return s.newModelLimitedSelectionResult(ctx, item.account)
					}
				}

				// 5. 所有路由账号槽位满，尝试返回等待计划（选择负载最低的）
				// 遍历找到第一个满足会话限制的账号
				for _, item := range routingAvailable {
					if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
						continue // 会话限制已满，尝试下一个
					}
					if s.debugModelRoutingEnabled() {
						logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed wait: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
					}
					return s.newSelectionResult(ctx, item.account, false, nil, newStickyWaitPlan(item.account.ID, item.account.Concurrency, cfg))
				}
				// 所有路由账号会话限制都已满，继续到 Layer 2 回退
			}
			// 路由列表中的账号都不可用（负载率 >= 100），继续到 Layer 2 回退
			logger.LegacyPrintf("service.gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal selection", requestedModel)
		}
	}

	// ============ Layer 1.5: 粘性会话（仅在无模型路由配置时生效） ============
	stickyBinding := scheduling.EvaluateStickyBinding(sessionHash, stickyAccountID, excludedIDs)
	if len(routingAccountIDs) == 0 && stickyBinding.Ready() {
		accountID := stickyBinding.AccountID
		account, ok := accountByID[accountID]
		if ok {
			// 检查账户是否需要清理粘性会话绑定
			clearSticky := shouldClearStickySession(account, requestedModel)
			candidateDecision := scheduling.EvaluateStickyCandidate(stickyBinding, account.ID, clearSticky)
			if candidateDecision == scheduling.StickyClear {
				slog.Debug("sticky.layer1_5_no_routing_clear",
					"account_id", accountID,
					"reason", "should_clear_sticky_session",
					"session", shortSessionHash(sessionHash),
				)
				_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
			}

			// 注意：不再检查 isAccountInGroup，因为 accountByID 已经从按分组过滤的
			// accounts 列表构建，账号一定在分组内。而 scheduler snapshot 缓存
			// 反序列化后 AccountGroups 字段为空，导致 isAccountInGroup 永远返回 false。
			stickyPolicyDecision := scheduling.EvaluateCandidate(ctx, account, scheduling.Request{
				Platform:           platform,
				RequestedModel:     requestedModel,
				ExcludedIDs:        excludedIDs,
				AllowMixedPlatform: useMixed,
				Sticky:             true,
			}, s.gatewayCandidatePolicies()...)

			slog.Debug("sticky.layer1_5_no_routing_checks",
				"account_id", accountID,
				"session", shortSessionHash(sessionHash),
				"clear_sticky", clearSticky,
				"policy_allowed", stickyPolicyDecision.Allowed,
				"rejected_policy", stickyPolicyDecision.RejectedPolicy,
			)

			if candidateDecision == scheduling.StickyReady && stickyPolicyDecision.Allowed {
				result, err := s.tryAcquireAccountSlot(ctx, requestedModel, accountID, account.Concurrency)
				if err == nil && result.Acquired {
					// 会话数量限制检查
					if !s.checkAndRegisterSession(ctx, account, sessionHash) {
						result.ReleaseFunc() // 释放槽位，继续到 Layer 2
						slog.Debug("sticky.layer1_5_no_routing_miss",
							"account_id", accountID,
							"reason", "session_limit",
							"session", shortSessionHash(sessionHash),
						)
					} else {
						slog.Debug("sticky.layer1_5_no_routing_hit",
							"account_id", accountID,
							"session", shortSessionHash(sessionHash),
							"result", "slot_acquired",
						)
						if s.cache != nil {
							_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL)
						}
						return s.newAcquiredSelectionResult(ctx, account, result.ReleaseFunc)
					}
				} else if err == nil && result.ModelLimited {
					// 模型级并发预算已满：立即让路，不进入账号等待队列。
					return s.newModelLimitedSelectionResult(ctx, account)
				} else {
					slog.Debug("sticky.layer1_5_no_routing_slot_busy",
						"account_id", accountID,
						"session", shortSessionHash(sessionHash),
					)
				}

				waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
				if waitingCount < cfg.StickySessionMaxWaiting {
					// 会话数量限制检查（等待计划也需要占用会话配额）
					if !s.checkAndRegisterSession(ctx, account, sessionHash) {
						// 会话限制已满，继续到 Layer 2
					} else {
						slog.Debug("sticky.layer1_5_no_routing_hit",
							"account_id", accountID,
							"session", shortSessionHash(sessionHash),
							"result", "wait_plan",
						)
						return s.newSelectionResult(ctx, account, false, nil, newStickyWaitPlan(accountID, account.Concurrency, cfg))
					}
				}
			} else if candidateDecision == scheduling.StickyReady {
				slog.Debug("sticky.layer1_5_no_routing_miss",
					"account_id", accountID,
					"reason", "gate_check_failed",
					"session", shortSessionHash(sessionHash),
				)
			}
		} else {
			slog.Debug("sticky.layer1_5_no_routing_miss",
				"account_id", accountID,
				"reason", "account_not_in_map",
				"session", shortSessionHash(sessionHash),
			)
		}
	} else if len(routingAccountIDs) == 0 && sessionHash != "" {
		slog.Debug("sticky.layer1_5_no_routing_skip",
			"sticky_account_id", stickyAccountID,
			"is_excluded", stickyAccountID > 0 && isExcluded(stickyAccountID),
			"session", shortSessionHash(sessionHash),
			"reason", func() string {
				if stickyAccountID == 0 {
					return "no_sticky_binding"
				}
				return "sticky_account_excluded"
			}(),
		)
	}

	// ============ Layer 2: 负载感知选择 ============
	slog.Debug("sticky.layer2_fallback",
		"session", shortSessionHash(sessionHash),
		"sticky_account_id", stickyAccountID,
		"reason", "sticky_not_used_falling_back_to_load_balance",
		"total_accounts", len(accounts),
	)
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		candidates = append(candidates, &accounts[i])
	}
	policyDecision := s.evaluateGatewayCandidates(ctx, candidates, scheduling.Request{
		Platform:           platform,
		RequestedModel:     requestedModel,
		ExcludedIDs:        excludedIDs,
		AllowMixedPlatform: useMixed,
	})
	candidates = policyDecision.AllowedAccounts

	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	var loadMap map[int64]*AccountLoadInfo
	if s.concurrencyService != nil && cfg.LoadBatchEnabled {
		loadMap, err = s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	} else {
		err = errGatewayLoadSnapshotUnavailable
	}
	s.reportGatewayCandidateSnapshots(ctx, candidates, loadMap)
	selectionOrder := gatewayCompatibilitySelectionOrder(candidates, loadMap, err == nil, preferOAuth, cfg.PreferSoonestReset)
	if rolloutMode == scheduling.RolloutEnforce && err == nil {
		sharedOrder := s.gatewayEnforceSelectionOrder(
			ctx,
			platform,
			groupID,
			sessionHash,
			requestedModel,
			stickyAccountID,
			preferOAuth,
			cfg.PreferSoonestReset,
			accounts,
			policyDecision,
			loadMap,
		)
		selectionOrder = orderGatewayAvailableByIDs(selectionOrder, sharedOrder)
	}
	for _, selected := range selectionOrder {
		result, acquireErr := s.tryAcquireAccountSlot(ctx, requestedModel, selected.account.ID, selected.account.Concurrency)
		if acquireErr == nil && result.Acquired {
			if !s.checkAndRegisterSession(ctx, selected.account, sessionHash) {
				result.ReleaseFunc()
				continue
			}
			if sessionHash != "" && s.cache != nil {
				_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.account.ID, stickySessionTTL)
			}
			return s.newAcquiredSelectionResult(ctx, selected.account, result.ReleaseFunc)
		}
		if acquireErr == nil && result.ModelLimited {
			return s.newModelLimitedSelectionResult(ctx, selected.account)
		}
	}

	// ============ Layer 3: 兜底排队 ============
	s.sortCandidatesForFallback(candidates, preferOAuth, cfg.FallbackSelectionMode)
	for _, acc := range candidates {
		// 会话数量限制检查（等待计划也需要占用会话配额）
		if !s.checkAndRegisterSession(ctx, acc, sessionHash) {
			continue // 会话限制已满，尝试下一个账号
		}
		return s.newSelectionResult(ctx, acc, false, nil, newFallbackWaitPlan(acc.ID, acc.Concurrency, cfg))
	}
	return nil, ErrNoAvailableAccounts
}

func gatewayCompatibilitySelectionOrder(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
	loadAvailable bool,
	preferOAuth bool,
	preferSoonestReset bool,
) []accountWithLoad {
	candidates := make([]scheduling.LoadCandidate, 0, len(accounts))
	loadByID := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
		}
		loadByID[account.ID] = loadInfo
		candidates = append(candidates, scheduling.LoadCandidate{
			Account:  account,
			LoadRate: loadInfo.LoadRate,
		})
	}
	var prefer func(*Account) bool
	if preferOAuth {
		prefer = func(account *Account) bool { return account.Type == AccountTypeOAuth }
	}
	ordered := scheduling.OrderPriorityLoadLRU(candidates, scheduling.PriorityLoadOrderOptions{
		UseLoad:            loadAvailable,
		PreferSoonestReset: loadAvailable && preferSoonestReset,
		Prefer:             prefer,
	})
	result := make([]accountWithLoad, 0, len(ordered))
	for _, candidate := range ordered {
		result = append(result, accountWithLoad{
			account:  candidate.Account,
			loadInfo: loadByID[candidate.Account.ID],
		})
	}
	return result
}

func (s *GatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	return schedulingConfigFrom(s.cfg)
}

func (s *GatewayService) withGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
	}
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && existing != nil && existing.ID == group.ID && IsGroupContextValid(existing) {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, group)
}

func (s *GatewayService) groupFromContext(ctx context.Context, groupID int64) *Group {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group
	}
	return nil
}

func (s *GatewayService) resolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group := s.groupFromContext(ctx, groupID); group != nil {
		return group, nil
	}
	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
	}
	return group, nil
}

func (s *GatewayService) ResolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	return s.resolveGroupByID(ctx, groupID)
}

func (s *GatewayService) resolveGatewayGroup(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, nil, nil
	}

	currentID := *groupID
	visited := map[int64]struct{}{}
	for {
		if _, seen := visited[currentID]; seen {
			return nil, nil, fmt.Errorf("fallback group cycle detected")
		}
		visited[currentID] = struct{}{}

		group, err := s.resolveGroupByID(ctx, currentID)
		if err != nil {
			return nil, nil, err
		}

		if !group.ClaudeCodeOnly || IsClaudeCodeClient(ctx) {
			return group, &currentID, nil
		}

		if group.FallbackGroupID == nil {
			return nil, nil, ErrClaudeCodeOnly
		}
		currentID = *group.FallbackGroupID
	}
}

// checkClaudeCodeRestriction 检查分组的 Claude Code 客户端限制
// 如果分组启用了 claude_code_only 且请求不是来自 Claude Code 客户端：
//   - 有降级分组：返回降级分组的 ID
//   - 无降级分组：返回 ErrClaudeCodeOnly 错误
func (s *GatewayService) checkClaudeCodeRestriction(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, groupID, nil
	}

	// 强制平台模式不检查 Claude Code 限制
	if forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string); hasForcePlatform && forcePlatform != "" {
		return nil, groupID, nil
	}

	group, resolvedID, err := s.resolveGatewayGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}

	return group, resolvedID, nil
}

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64, group *Group) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
	}
	if group != nil {
		return group.Platform, false, nil
	}
	if platform, ok := ctx.Value(gatewayPlatformHintContextKey{}).(string); ok && platform != "" {
		return platform, false, nil
	}
	if groupID != nil {
		group, err := s.resolveGroupByID(ctx, *groupID)
		if err != nil {
			return "", false, err
		}
		return group.Platform, false, nil
	}
	return PlatformAnthropic, false, nil
}

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	repoPlatforms := []string{platform}
	var repoFilter func(Account) bool
	if useMixed {
		repoPlatforms = []string{platform, PlatformAntigravity}
		repoFilter = func(account Account) bool {
			return account.Platform != PlatformAntigravity || account.IsMixedSchedulingEnabled()
		}
	}
	var snapshot scheduling.AccountListSnapshot
	if s.schedulerSnapshot != nil {
		snapshot = s.schedulerSnapshot
	}
	result, err := scheduling.ListAccounts(ctx, snapshot, s.accountRepo, scheduling.AccountListRequest{
		GroupID:          groupID,
		Platform:         platform,
		HasForcePlatform: hasForcePlatform,
		UseMixed:         useMixed,
		RepoPlatforms:    repoPlatforms,
		RepoFilter:       repoFilter,
	})
	if err != nil {
		if !result.FromSnapshot {
			slog.Debug("account_scheduling_list_failed",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"error", err)
		}
		return result.Accounts, result.UseMixed, err
	}
	accounts := result.Accounts
	if result.FromSnapshot {
		slog.Debug("account_scheduling_list_snapshot",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"use_mixed", result.UseMixed,
			"count", len(accounts))
	} else if result.UseMixed {
		slog.Debug("account_scheduling_list_mixed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"raw_count", result.RawCount,
			"filtered_count", len(accounts))
	} else {
		slog.Debug("account_scheduling_list_single",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"count", len(accounts))
	}
	logSchedulingAccountDetails(ctx, accounts)
	return accounts, result.UseMixed, nil
}

func logSchedulingAccountDetails(ctx context.Context, accounts []Account) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}
	for _, account := range accounts {
		slog.Debug("account_scheduling_account_detail",
			"account_id", account.ID,
			"name", account.Name,
			"platform", account.Platform,
			"type", account.Type,
			"status", account.Status,
			"tls_fingerprint", account.IsTLSFingerprintEnabled())
	}
}

// IsSingleAntigravityAccountGroup 检查指定分组是否只有一个 antigravity 平台的可调度账号。
// 用于 Handler 层在首次请求时提前设置 SingleAccountRetry context，
// 避免单账号分组收到 503 时错误地设置模型限流标记导致后续请求连续快速失败。
func (s *GatewayService) IsSingleAntigravityAccountGroup(ctx context.Context, groupID *int64) bool {
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAntigravity, true)
	if err != nil {
		return false
	}
	return len(accounts) == 1
}

func (s *GatewayService) tryAcquireAccountSlot(ctx context.Context, requestedModel string, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	return tryAcquireAccountSlot(ctx, s.concurrencyService, requestedModel, accountID, maxConcurrency)
}

type usageLogWindowStatsBatchProvider interface {
	GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error)
}

type windowCostPrefetchContextKeyType struct{}

var windowCostPrefetchContextKey = windowCostPrefetchContextKeyType{}

func windowCostFromPrefetchContext(ctx context.Context, accountID int64) (float64, bool) {
	if ctx == nil || accountID <= 0 {
		return 0, false
	}
	m, ok := ctx.Value(windowCostPrefetchContextKey).(map[int64]float64)
	if !ok || len(m) == 0 {
		return 0, false
	}
	v, exists := m[accountID]
	return v, exists
}

func (s *GatewayService) withWindowCostPrefetch(ctx context.Context, accounts []Account) context.Context {
	if ctx == nil || len(accounts) == 0 || s.sessionLimitCache == nil || s.usageLogRepo == nil {
		return ctx
	}

	accountByID := make(map[int64]*Account)
	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
			continue
		}
		if account.GetWindowCostLimit() <= 0 {
			continue
		}
		accountByID[account.ID] = account
		accountIDs = append(accountIDs, account.ID)
	}
	if len(accountIDs) == 0 {
		return ctx
	}

	costs := make(map[int64]float64, len(accountIDs))
	cacheValues, err := s.sessionLimitCache.GetWindowCostBatch(ctx, accountIDs)
	if err == nil {
		for accountID, cost := range cacheValues {
			costs[accountID] = cost
		}
		windowCostPrefetchCacheHitTotal.Add(int64(len(cacheValues)))
	} else {
		windowCostPrefetchErrorTotal.Add(1)
		logger.LegacyPrintf("service.gateway", "window_cost batch cache read failed: %v", err)
	}
	cacheMissCount := len(accountIDs) - len(costs)
	if cacheMissCount < 0 {
		cacheMissCount = 0
	}
	windowCostPrefetchCacheMissTotal.Add(int64(cacheMissCount))

	missingByStart := make(map[int64][]int64)
	startTimes := make(map[int64]time.Time)
	for _, accountID := range accountIDs {
		if _, ok := costs[accountID]; ok {
			continue
		}
		account := accountByID[accountID]
		if account == nil {
			continue
		}
		startTime := account.GetCurrentWindowStartTime()
		startKey := startTime.Unix()
		missingByStart[startKey] = append(missingByStart[startKey], accountID)
		startTimes[startKey] = startTime
	}
	if len(missingByStart) == 0 {
		return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
	}

	batchReader, hasBatch := s.usageLogRepo.(usageLogWindowStatsBatchProvider)
	for startKey, ids := range missingByStart {
		startTime := startTimes[startKey]

		if hasBatch {
			windowCostPrefetchBatchSQLTotal.Add(1)
			queryStart := time.Now()
			statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, ids, startTime)
			if err == nil {
				slog.Debug("window_cost_batch_query_ok",
					"accounts", len(ids),
					"window_start", startTime.Format(time.RFC3339),
					"duration_ms", time.Since(queryStart).Milliseconds())
				for _, accountID := range ids {
					stats := statsByAccount[accountID]
					cost := 0.0
					if stats != nil {
						cost = stats.StandardCost
					}
					costs[accountID] = cost
					_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
				}
				continue
			}
			windowCostPrefetchErrorTotal.Add(1)
			logger.LegacyPrintf("service.gateway", "window_cost batch db query failed: start=%s err=%v", startTime.Format(time.RFC3339), err)
		}

		// 回退路径：缺少批量仓储能力或批量查询失败时，按账号单查（失败开放）。
		windowCostPrefetchFallbackTotal.Add(int64(len(ids)))
		for _, accountID := range ids {
			stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, accountID, startTime)
			if err != nil {
				windowCostPrefetchErrorTotal.Add(1)
				continue
			}
			cost := stats.StandardCost
			costs[accountID] = cost
			_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
		}
	}

	return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
}

// isAccountSchedulableForQuota 检查账号是否在配额限制内
// 适用于配置了 quota_limit 的 apikey 类型账号
func (s *GatewayService) accountPassesQuota(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return true
	}
	return !account.IsQuotaExceeded()
}

// isAccountSchedulableForWindowCost 检查账号是否可根据窗口费用进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
// 返回 true 表示可调度，false 表示不可调度
func (s *GatewayService) accountPassesWindowCost(ctx context.Context, account *Account, isSticky bool) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	limit := account.GetWindowCostLimit()
	if limit <= 0 {
		return true // 未启用窗口费用限制
	}

	// 尝试从缓存获取窗口费用
	var currentCost float64
	if cost, ok := windowCostFromPrefetchContext(ctx, account.ID); ok {
		currentCost = cost
		goto checkSchedulability
	}
	if s.sessionLimitCache != nil {
		if cost, hit, err := s.sessionLimitCache.GetWindowCost(ctx, account.ID); err == nil && hit {
			currentCost = cost
			goto checkSchedulability
		}
	}

	// 缓存未命中，从数据库查询
	{
		// 使用统一的窗口开始时间计算逻辑（考虑窗口过期情况）
		startTime := account.GetCurrentWindowStartTime()

		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
		if err != nil {
			// 失败开放：查询失败时允许调度
			return true
		}

		// 使用标准费用（不含账号倍率）
		currentCost = stats.StandardCost

		// 设置缓存（忽略错误）
		if s.sessionLimitCache != nil {
			_ = s.sessionLimitCache.SetWindowCost(ctx, account.ID, currentCost)
		}
	}

checkSchedulability:
	schedulability := account.CheckWindowCostSchedulability(currentCost)

	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// rpmPrefetchContextKey is the context key for prefetched RPM counts.
type rpmPrefetchContextKeyType struct{}

var rpmPrefetchContextKey = rpmPrefetchContextKeyType{}

func rpmFromPrefetchContext(ctx context.Context, accountID int64) (int, bool) {
	if v, ok := ctx.Value(rpmPrefetchContextKey).(map[int64]int); ok {
		count, found := v[accountID]
		return count, found
	}
	return 0, false
}

// withRPMPrefetch 批量预取所有候选账号的 RPM 计数
func (s *GatewayService) withRPMPrefetch(ctx context.Context, accounts []Account) context.Context {
	if s.rpmCache == nil {
		return ctx
	}

	var ids []int64
	for i := range accounts {
		if accounts[i].IsAnthropicOAuthOrSetupToken() && accounts[i].GetBaseRPM() > 0 {
			ids = append(ids, accounts[i].ID)
		}
	}
	if len(ids) == 0 {
		return ctx
	}

	counts, err := s.rpmCache.GetRPMBatch(ctx, ids)
	if err != nil {
		return ctx // 失败开放
	}
	return context.WithValue(ctx, rpmPrefetchContextKey, counts)
}

// isAccountSchedulableForRPM 检查账号是否可根据 RPM 进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
func (s *GatewayService) accountPassesRPM(ctx context.Context, account *Account, isSticky bool) bool {
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}
	baseRPM := account.GetBaseRPM()
	if baseRPM <= 0 {
		return true
	}

	// 尝试从预取缓存获取
	var currentRPM int
	if count, ok := rpmFromPrefetchContext(ctx, account.ID); ok {
		currentRPM = count
	} else if s.rpmCache != nil {
		if count, err := s.rpmCache.GetRPM(ctx, account.ID); err == nil {
			currentRPM = count
		}
		// 失败开放：GetRPM 错误时允许调度
	}

	schedulability := account.CheckRPMSchedulability(currentRPM)
	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// IncrementAccountRPM increments the RPM counter for the given account.
// 已知 TOCTOU 竞态：调度时读取 RPM 计数与此处递增之间存在时间窗口，
// 高并发下可能短暂超出 RPM 限制。这是与 WindowCost 一致的 soft-limit
// 设计权衡——可接受的少量超额优于加锁带来的延迟和复杂度。
func (s *GatewayService) IncrementAccountRPM(ctx context.Context, accountID int64) error {
	if s.rpmCache == nil {
		return nil
	}
	_, err := s.rpmCache.IncrementRPM(ctx, accountID)
	return err
}

// checkAndRegisterSession 检查并注册会话，用于会话数量限制
// 仅适用于 Anthropic OAuth/SetupToken 账号
// sessionID: 会话标识符（使用粘性会话的 hash）
// 返回 true 表示允许（在限制内或会话已存在），false 表示拒绝（超出限制且是新会话）
func (s *GatewayService) checkAndRegisterSession(ctx context.Context, account *Account, sessionID string) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	maxSessions := account.GetMaxSessions()
	if maxSessions <= 0 || sessionID == "" {
		return true // 未启用会话限制或无会话ID
	}

	if s.sessionLimitCache == nil {
		return true // 缓存不可用时允许通过
	}

	idleTimeout := time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute

	allowed, err := s.sessionLimitCache.RegisterSession(ctx, account.ID, sessionID, maxSessions, idleTimeout)
	if err != nil {
		// 失败开放：缓存错误时允许通过
		return true
	}
	return allowed
}

func (s *GatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	return newHydratedSelectionResult(ctx, s.schedulerSnapshot, account, "gateway", acquired, release, waitPlan)
}

// newAcquiredSelectionResult 构造已抢到并发槽的选择结果；若 hydrate 失败则立即释放槽位，
// 避免槽位泄漏直到 Redis TTL 过期（与 OpenAIGatewayService.newAcquiredSelectionResult 对称）。
func (s *GatewayService) newAcquiredSelectionResult(ctx context.Context, account *Account, release func()) (*AccountSelectionResult, error) {
	return newHydratedAcquiredResult(ctx, s.schedulerSnapshot, account, "gateway", release)
}

// newModelLimitedSelectionResult 构造模型级预算已满的拒绝结果。
// WaitPlan 恒为 nil，Handler 层据此直接 429 让路，不进入账号等待队列。
func (s *GatewayService) newModelLimitedSelectionResult(ctx context.Context, account *Account) (*AccountSelectionResult, error) {
	return newHydratedModelLimitedResult(ctx, s.schedulerSnapshot, account, "gateway")
}

func sortAccountsByPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	var prefer func(*Account) bool
	if preferOAuth {
		prefer = func(account *Account) bool { return account.Type == AccountTypeOAuth }
	}
	scheduling.SortPriorityLRU(accounts, prefer)
}

// sortCandidatesForFallback 根据配置选择排序策略
// mode: "last_used"(按最后使用时间) 或 "random"(随机)
func (s *GatewayService) sortCandidatesForFallback(accounts []*Account, preferOAuth bool, mode string) {
	if mode == "random" {
		// 先按优先级排序，然后在同优先级内随机打乱
		sortAccountsByPriorityOnly(accounts, preferOAuth)
		shuffleWithinPriority(accounts)
	} else {
		// 默认按最后使用时间排序
		sortAccountsByPriorityAndLastUsed(accounts, preferOAuth)
	}
}

// sortAccountsByPriorityOnly 仅按优先级排序
func sortAccountsByPriorityOnly(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if preferOAuth && a.Type != b.Type {
			return a.Type == AccountTypeOAuth
		}
		return false
	})
}

// shuffleWithinPriority 在同优先级内随机打乱顺序
func shuffleWithinPriority(accounts []*Account) {
	if len(accounts) <= 1 {
		return
	}
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	start := 0
	for start < len(accounts) {
		priority := accounts[start].Priority
		end := start + 1
		for end < len(accounts) && accounts[end].Priority == priority {
			end++
		}
		// 对 [start, end) 范围内的账户随机打乱
		if end-start > 1 {
			r.Shuffle(end-start, func(i, j int) {
				accounts[start+i], accounts[start+j] = accounts[start+j], accounts[start+i]
			})
		}
		start = end
	}
}

type selectionFailureStats struct {
	Total              int
	Eligible           int
	Excluded           int
	Unschedulable      int
	PlatformFiltered   int
	ModelUnsupported   int
	ModelRateLimited   int
	SamplePlatformIDs  []int64
	SampleMappingIDs   []int64
	SampleRateLimitIDs []string
}

type selectionFailureDiagnosis struct {
	Category string
	Detail   string
}

func (s *GatewayService) collectSelectionFailureStats(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := selectionFailureStats{
		Total: len(accounts),
	}

	for i := range accounts {
		acc := &accounts[i]
		diagnosis := s.diagnoseSelectionFailure(ctx, acc, requestedModel, platform, excludedIDs, allowMixedScheduling)
		switch diagnosis.Category {
		case "excluded":
			stats.Excluded++
		case "unschedulable":
			stats.Unschedulable++
		case "platform_filtered":
			stats.PlatformFiltered++
			stats.SamplePlatformIDs = appendSelectionFailureSampleID(stats.SamplePlatformIDs, acc.ID)
		case "model_unsupported":
			stats.ModelUnsupported++
			stats.SampleMappingIDs = appendSelectionFailureSampleID(stats.SampleMappingIDs, acc.ID)
		case "model_rate_limited":
			stats.ModelRateLimited++
			remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
			stats.SampleRateLimitIDs = appendSelectionFailureRateSample(stats.SampleRateLimitIDs, acc.ID, remaining)
		default:
			stats.Eligible++
		}
	}

	return stats
}

func (s *GatewayService) diagnoseSelectionFailure(
	ctx context.Context,
	acc *Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureDiagnosis {
	if acc == nil {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "account_nil"}
	}
	decision := scheduling.EvaluateCandidate(ctx, acc, scheduling.Request{
		Platform:           platform,
		RequestedModel:     requestedModel,
		ExcludedIDs:        excludedIDs,
		AllowMixedPlatform: allowMixedScheduling,
	}, s.gatewayCandidatePolicies()...)
	switch decision.RejectedPolicy {
	case "not_excluded":
		return selectionFailureDiagnosis{Category: "excluded"}
	case "schedulable":
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "generic_unschedulable"}
	case "gateway_platform":
		return selectionFailureDiagnosis{
			Category: "platform_filtered",
			Detail:   fmt.Sprintf("account_platform=%s requested_platform=%s", acc.Platform, strings.TrimSpace(platform)),
		}
	case "model_capability":
		return selectionFailureDiagnosis{
			Category: "model_unsupported",
			Detail:   fmt.Sprintf("model=%s", requestedModel),
		}
	case "schedulable_for_model":
		remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
		return selectionFailureDiagnosis{
			Category: "model_rate_limited",
			Detail:   fmt.Sprintf("remaining=%s", remaining),
		}
	}
	return selectionFailureDiagnosis{Category: "eligible"}
}

func appendSelectionFailureSampleID(samples []int64, id int64) []int64 {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, id)
}

func appendSelectionFailureRateSample(samples []string, accountID int64, remaining time.Duration) []string {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, fmt.Sprintf("%d(%s)", accountID, remaining))
}

// isModelSupportedByAccountWithContext 根据账户平台检查模型支持（带 context）
// 对于 Antigravity 平台，会先获取映射后的最终模型名（包括 thinking 后缀）再检查支持
func (s *GatewayService) isModelSupportedByAccountWithContext(ctx context.Context, account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		// 使用与转发阶段一致的映射逻辑：自定义映射优先 → 默认映射兜底
		mapped := mapAntigravityModel(account, requestedModel)
		if mapped == "" {
			return false
		}
		// 应用 thinking 后缀后检查最终模型是否在账号映射中
		if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
			finalModel := applyThinkingModelSuffix(mapped, enabled)
			if finalModel == mapped {
				return true // thinking 后缀未改变模型名，映射已通过
			}
			return account.IsModelSupported(finalModel)
		}
		return true
	}
	return s.isModelSupportedByAccount(account, requestedModel)
}

// isModelSupportedByAccount 根据账户平台检查模型支持（无 context，用于非 Antigravity 平台）
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	// OpenAI 透传模式：仅替换认证，允许所有模型
	if account.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
		return true
	}
	// OAuth/SetupToken 账号使用 Anthropic 标准映射（短ID → 长ID）
	if account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		if account.Type == AccountTypeServiceAccount {
			requestedModel = normalizeVertexAnthropicModelID(claude.NormalizeModelID(requestedModel))
		} else {
			requestedModel = claude.NormalizeModelID(requestedModel)
		}
	}
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
}
