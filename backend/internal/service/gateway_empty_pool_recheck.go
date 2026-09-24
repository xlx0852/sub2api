package service

import (
	"context"
	"errors"
	"log/slog"
)

// 本文件补齐一条调度不对称：OpenAI 协议路径在"选号返回 ErrNoAvailableAccounts"时会强制回 DB
// 重建 bucket 快照并重试一次（见 openai_account_scheduler.go 的 *_empty_pool_db_recheck 事件），
// 而网关路径（Anthropic /v1/messages、Gemini、Antigravity）过去只读快照，第一跳看到空池就直接失败。
//
// 关键约束（改动前请先读）：
//
//  1. 只在"空池"时复核，不在"账号忙"时复核。池/候选集任一为空才叫空池；候选非空但抢不到并发槽位
//     或只能进等待计划时复核没有任何意义，只会放大 DB 读（见 gatewaySelectionPoolStats.EmptyPool）。
//  2. 只在调用方没有排除账号时复核。failover 已经明确"这次不要这些账号"，复核后重试只会拿回同一批
//     被排除的账号。
//  3. 不新增开关、不新增节流：复核走 SchedulerSnapshotService.RefreshSchedulableAccounts，沿用
//     per-bucket 节流 schedulerForcedRefreshGap 与 DB fallback 限流器；窗口内重复调用会返回
//     ErrSchedulerFallbackLimited，此时静默放弃（与 OpenAI 路径一致，不算失败）。
//  4. 复核成功后只重试一次选号；仍失败时保留原有错误分类（404 model_not_found / 503），不得假装成功。
//  5. 日志只含 group_id / platform / model / 计数，禁止带上游报文或凭据。

// gatewaySelectionPoolStats 记录一次网关选号尝试的池规模。
//
// EmptyPool 为 true 表示"这一跳看不到可用账号"，是强制 DB 复核的唯一依据。
type gatewaySelectionPoolStats struct {
	Platform         string
	HasForcePlatform bool
	// PoolSize 为硬门槛过滤前的候选账号数（快照或 DB 列表长度）。
	PoolSize int
	// CandidateCount 为通过硬门槛后的候选数。
	CandidateCount int
	// EmptyPool 表示池或候选集为空（区别于"候选存在但拿不到槽位"）。
	EmptyPool bool
}

// recheckEmptyGatewayPool 在空池时强制回 DB 复核一次该 bucket。
// 返回 true 表示复核已执行（调用方应重试一次选号）。
func (s *GatewayService) recheckEmptyGatewayPool(
	ctx context.Context,
	groupID *int64,
	stats gatewaySelectionPoolStats,
	requestedModel string,
) bool {
	if s == nil || s.schedulerSnapshot == nil {
		return false
	}
	refreshed, _, err := s.schedulerSnapshot.RefreshSchedulableAccounts(ctx, groupID, stats.Platform, stats.HasForcePlatform)
	if err != nil {
		if !errors.Is(err, ErrSchedulerFallbackLimited) {
			slog.Warn("gateway_scheduler_empty_pool_db_recheck_failed",
				"platform", stats.Platform,
				"group_id", derefGroupID(groupID),
				"model", requestedModel,
				"error", err,
			)
		}
		return false
	}
	slog.Warn("gateway_scheduler_empty_pool_db_recheck",
		"platform", stats.Platform,
		"group_id", derefGroupID(groupID),
		"model", requestedModel,
		"snapshot_pool_size", stats.PoolSize,
		"snapshot_candidate_count", stats.CandidateCount,
		"db_candidate_count", len(refreshed),
	)
	return true
}

// selectGatewayAccountWithEmptyPoolRecheck 编排"空池 → 强制 DB 复核 → 重试一次"。
//
// attempt 必须是"单次选号尝试"：成功时返回结果，失败时返回错误；空池失败必须把
// gatewaySelectionPoolStats.EmptyPool 置为 true 并回传该次尝试的池规模。
func (s *GatewayService) selectGatewayAccountWithEmptyPoolRecheck(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	excludedIDs map[int64]struct{},
	attempt func() (*AccountSelectionResult, gatewaySelectionPoolStats, error),
) (*AccountSelectionResult, error) {
	result, stats, err := attempt()
	if err == nil || !errors.Is(err, ErrNoAvailableAccounts) {
		return result, err
	}
	// 有排除账号（failover）或账号只是忙：复核无意义，保持既有行为。
	if len(excludedIDs) > 0 || !stats.EmptyPool {
		return nil, err
	}
	if !s.recheckEmptyGatewayPool(ctx, groupID, stats, requestedModel) {
		return nil, err
	}
	retried, retryStats, retryErr := attempt()
	if retryErr != nil {
		slog.Warn("gateway_scheduler_empty_pool_db_recheck_unrecovered",
			"platform", retryStats.Platform,
			"group_id", derefGroupID(groupID),
			"model", requestedModel,
			"db_pool_size", retryStats.PoolSize,
			"db_candidate_count", retryStats.CandidateCount,
			"error", retryErr,
		)
		return nil, retryErr
	}
	return retried, nil
}
