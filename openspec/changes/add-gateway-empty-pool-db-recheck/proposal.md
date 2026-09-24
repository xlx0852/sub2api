# Change: 网关（Anthropic/Gemini/Antigravity）空池时强制回 DB 复核

## Why

生产已在真实流量上暴露一个调度不对称：

- **OpenAI 协议路径**在"选号返回 `ErrNoAvailableAccounts`"时会强制回 DB 重建该 bucket 快照并重试一次
  （`openai_account_scheduler.go`：`openai_scheduler_empty_pool_db_recheck` / `..._unrecovered` / `..._failed`）。
- **网关路径（Anthropic `/v1/messages`、Gemini、Antigravity）没有这个复核**：`gatewaySchedulingStrategy`
  直接落到 `GatewayService.selectAccountWithLoadAwarenessStrategy`，它只读调度快照；一旦快照短暂缺号，
  第一跳就带着"空候选池"返回。

2026-09-25 02:25:38–02:25:39 (+08) 的实测证据：

```
gateway_scheduler_no_candidates_reject_trace
  group_id=34 platform=anthropic model=deepseek-flash
  reject_trace=[{81: model_capability}, {139: model_capability}]
```

同一时刻账号 140（唯一认领 `deepseek-flash` / `deepseek-v4-flash` / `deepseek-v4-flash-vision-exp`
的账号）在池中缺席，三个请求各自返回 `404 Model "…" is not supported by any configured account in this
group`；约 3 分钟后快照写入（`sched:acc:140` 的 `UpdatedAt=02:26:51`）后自行恢复，同模型立即又能 200。
账号本身始终 `status=active`、`schedulable=true`、无 `rate_limited_at`。

后果有两层：

1. **用户可见的错误分类是错的**：池子只是"这一跳看不到"，却按"没有任何账号配置该模型"返回 404
   `model_not_found`，误导运维去改 `model_mapping`。
2. **恢复被推迟到下一次快照写入**（本例约 3 分钟），期间该模型只能靠别的账号（139）兜，或直接失败。

## What Changes

- 在网关选号路径引入"空池 → 强制 DB 复核 → 重试一次"的编排，与 OpenAI 路径语义对齐：
  - 触发条件：首次尝试返回 `ErrNoAvailableAccounts`，**且**该次尝试的池/候选集为空
    （`PoolSize == 0` 或 `CandidateCount == 0`），**且**调用方没有排除账号（failover 场景不复核）。
  - 复核动作：调用 `SchedulerSnapshotService.RefreshSchedulableAccounts`（既有 per-bucket 节流
    `schedulerForcedRefreshGap` 与 DB fallback 限流器照旧生效）。
  - 复核成功则整轮选号重试一次；仍失败时保持原有错误分类与状态码，并记录 `..._unrecovered`。
  - 新增事件：`gateway_scheduler_empty_pool_db_recheck`、`gateway_scheduler_empty_pool_db_recheck_unrecovered`、
    `gateway_scheduler_empty_pool_db_recheck_failed`（与 OpenAI 命名对齐，便于同一条告警规则覆盖两侧）。
- 明确不改变的行为：账号只是忙（候选非空但抢不到槽位/无等待计划）、无快照服务、有排除账号、
  渠道定价/Claude Code 限制等前置拒绝，一律**不**触发复核。
- 事件字段只含 group/platform/model 与计数，不含凭据。

## Impact

- 影响 specs：`account-selection`（新增一条 requirement）。
- 影响代码：`internal/service/gateway_scheduling.go`（拆出单次尝试 + 增加编排）、
  新增 `internal/service/gateway_empty_pool_recheck.go`。
- 性能：仅在"空池"这一原本就会失败/降级的路径上，每个 bucket 最多每 `schedulerForcedRefreshGap`(1s)
  多一次 DB 读，且受既有 DB fallback 限流器约束；正常选号路径零额外开销。
- 风险：空池时多一次重试意味着同一请求可能多走一遍候选遍历（无副作用：该路径未持有并发槽位、未写 sticky）。
