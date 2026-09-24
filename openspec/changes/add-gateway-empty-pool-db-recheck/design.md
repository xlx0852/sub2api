# Design: 网关空池 DB 复核

## D1 触发条件必须是"空池"，不是"选不到"

`selectAccountWithLoadAwarenessStrategy` 会在三种情况下返回 `ErrNoAvailableAccounts`：

| 情况 | 池 | 候选 | 是否复核 | 理由 |
|---|---|---|---|---|
| A 快照/DB 列表为空 | 0 | 0 | ✅ | 与 OpenAI 路径一致；可能是快照缺号 |
| B 池非空但无账号通过硬门槛（模型能力等） | >0 | 0 | ✅ | 生产实测场景：池里只有 81/139，140 缺席 |
| C 池与候选都非空，但抢不到并发槽位/无等待计划 | >0 | >0 | ❌ | 账号存在只是忙；复核不会改变结果，且会放大 DB 读 |

因此内部尝试函数要回传池统计，由编排层判断，而不是只看错误类型。

## D2 只在无排除账号时复核

与 OpenAI 路径一致（`len(req.ExcludedIDs) == 0`）。failover 已经明确"这次不要哪些账号"，
复核后重试会拿回同一批被排除的账号，只会浪费一次 DB 读。

## D3 复用既有节流与限流，不新增开关

`SchedulerSnapshotService.RefreshSchedulableAccounts` 已经具备：

- per-bucket 节流 `schedulerForcedRefreshGap`（1s）：窗口内重复调用返回 `ErrSchedulerFallbackLimited`；
- `guardFallback` + `fallbackLimiter`（`DbFallbackMaxQPS`）控制 DB 回退速率；
- 失败时返回错误但不影响调用方继续走原有错误路径。

`ErrSchedulerFallbackLimited` 视为"复核被节流"，**静默**返回 false（不算失败、不打 `_failed` 日志），
与 OpenAI 路径的判断一致。

## D4 可测性：把编排抽成显式函数

为了让"空池 → 复核 → 重试一次"可以被单测覆盖（不必构造完整网关依赖：group/cache/concurrency/channel 等），
编排下沉为：

```go
func (s *GatewayService) selectGatewayAccountWithEmptyPoolRecheck(
    ctx context.Context,
    groupID *int64,
    requestedModel string,
    excludedIDs map[int64]struct{},
    attempt func() (*AccountSelectionResult, gatewaySelectionPoolStats, error),
) (*AccountSelectionResult, error)
```

`selectAccountWithLoadAwarenessStrategy` 只负责提供 `attempt` 闭包（调用单次尝试函数）。
复核本身抽为 `recheckEmptyGatewayPool`，可直接对 `*SchedulerSnapshotService` 的假 cache/repo 断言
"是否真的回 DB"。

## D5 日志与可观测

事件名与 OpenAI 侧对齐，同一告警规则可按前缀 `*_empty_pool_db_recheck` 覆盖两条路径：

- `gateway_scheduler_empty_pool_db_recheck`：snapshot_pool_size / snapshot_candidate_count / db_candidate_count
- `gateway_scheduler_empty_pool_db_recheck_unrecovered`：复核后重试仍失败
- `gateway_scheduler_empty_pool_db_recheck_failed`：复核本身报错（节流除外）

字段仅 group_id / platform / model / 计数，无凭据、无上游报文。

## D6 不做的事

- 不改 `ErrNoAvailableAccounts` → 404/503 的分类逻辑（`classifyNoAccountError` 仍按 DB 视角判定；
  复核成功后不会再走到它）。
- 不给快照写入加同步等待，不改 outbox/全量重建节奏。
- 不引入新的配置开关（默认行为，与 OpenAI 路径一致）。
