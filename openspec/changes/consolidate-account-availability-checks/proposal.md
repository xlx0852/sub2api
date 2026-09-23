# Change: 收敛账号可用性判定为单一真相源

## Why

本次账号 69/89/132 排障反复踩同一类坑：**"账号当前能不能被调度"这个判断散落在多个互相不知情的位置**，且关键的过滤决策完全不可观测，导致"有号但报无号""有票但 502""代理断了却反复清票"三类线上事故只能靠人肉逐字段排查。

具体暴露的缺陷：

1. **过滤链不可观测**：`selectByLoadBalance` 空候选时没有任何"哪个号被哪条 policy 拒绝"的现场证据，`policy_trace` 只在 shadow 模式才收集，生产排查只能逐个手查 `schedulable`/`model_mapping`/冷却字段。

2. **transport 错误被误当上游 5xx 清票**：`handleOpenAIUpstreamTransportError` 把 proxy/DNS/TCP/TLS 失败包装成 `UpstreamFailoverError{StatusCode:502}`，handler 透传给 `NoteOpenAICodexTurnStateUpstreamResult`，被计入"连续 5xx → invalidate 票 + 重抓"。69 号 SOCKS 代理超时期间票被反复清掉重抓，但票根本没失效——是代理死了。

3. **无候选错误不分层级**：`no available accounts` 把"平台级无候选"和"模型级无候选（组内有健康号但都未映射该模型）"混在一起，误导排查方向。

## What Changes

### A. 空候选时输出 policy 拒绝明细（已落地）

- `selectByLoadBalance` 的过滤循环始终收集 `CandidatePolicyTrace`（不再仅 shadow 模式）；当 `filtered` 为空时输出 `openai_scheduler_no_candidates_reject_trace`，逐账号列出被拒绝的 policy 名。
- 正常路径仅多一次切片追加，零行为变化。

### B. transport 错误与票失效解耦（已落地）

- `UpstreamFailoverError` 增加 `TransportOnly` 标记；`handleOpenAIUpstreamTransportError` 置 true。
- `NoteOpenAICodexTurnStateUpstreamResult` 对 `TransportOnly` 直接返回，不计入连续 5xx、不 invalidate 票、不触发重抓。
- 注意：transport 错误**仍**计入账号调度失败率（`ReportOpenAIAccountScheduleResult`），因为代理坏掉的号确实应降权——这与"清票"是两个独立决策，本 change 只纠正后者。

### C. 无候选错误分级（已落地）

- `noAvailableOpenAIAccountError` 增加 `reason` 字段：`platform_empty`（快照层无候选）/ `filtered_all`（前置过滤全刷）/ `model_filtered`（selectionOrder 为空）/ `acquire_or_recheck_failed`（复核/acquire 阶段全失败），错误信息与日志可定位缺失层级。

## 评估后不在本 change 做的事

- **拉黑合一（删 `openaiAccountRuntimeBlockUntil` 改纯 DB 冷却）**：内存块与 `temp_unschedulable_until` 是**互补分层**而非重复——内存管"DB 落盘前的进程内即时保护"（避免刚 429 又选中），DB 管"跨实例持久+审计"。删内存层会开"刚失败又选中"窗口；429 的秒级冷却改成同步 DB 写是负优化。本 change 已通过 reject_trace 让 `runtime_blocked` 拦截可观测，覆盖了该类事故的实际排查痛点。
- **turn-state 票迁 Redis**：票生命周期与 `harden-openai-pool-outcome-sticky-stream` 的 outcome 分类/sticky re-eval 强耦合，现在迁移会与该 active change 撞车；待其收口后再单独立项。
- **合并 UserPin/SessionSticky/PreviousResponse 三层粘性**：`promote-unified-scoring-scheduler` 统一引擎时处理。
- 不改选号算法、打分权重、TopK、加权随机。

## Impact

- Affected specs: `account-availability`（新 capability）、`architecture-governance`（可观测性要求）
- Affected code:
  - `backend/internal/service/openai_account_scheduler.go`（reject trace + 错误分级）
  - `backend/internal/service/openai_gateway_scheduling.go`（错误类型加 reason）
  - `backend/internal/service/openai_upstream_transport_error.go`（TransportOnly）
  - `backend/internal/service/openai_codex_turn_state.go`（transport 跳过清票）
  - `backend/internal/handler/openai_gateway_handler.go` / `openai_chat_completions.go`（透传 TransportOnly）
  - `backend/internal/service/gateway_service.go`（failover 错误字段）
- **BREAKING**: 无；`noAvailableOpenAIAccountError.Error()` 追加了 `reason=` 后缀，属可观测性增强。
- Rollback: 两个 commit 独立可 revert，不改变调度决策结果。

## Non-Goals

见上文"评估后不在本 change 做的事"。
