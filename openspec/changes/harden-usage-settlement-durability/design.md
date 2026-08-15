## Context

当前生产链路并不是 `pipeline.StageSettle` 直接持久化账务：该阶段主要把 provider usage 组装成 `ForwardResult`，handler 随后把 `RecordUsage` 闭包提交到有界但非持久化的进程内 worker pool。`RecordUsage` 先调用 `usageBillingRepository.Apply` 提交账务幂等和金额/配额变更，再单独写 `usage_logs`。

现状已有三层保护：流式客户端断连后继续 drain 上游、worker 队列默认使用同步溢出兜底、数据库账务变更在单事务内按 `(request_id, api_key_id)` 幂等执行。这些保护能处理普通断连、队列拥塞和重复调用，但不能处理进程在内存 usage、内存队列或两个数据库提交之间退出。

`harden-usage-billing-admission` 计划引入请求前 reserve/capture/release。它能限制并发透支，并可让崩溃后的预留通过 TTL 恢复；它不能重建终态 token/cost，也不能替代 durable settlement event。本 change 不依赖其未完成实现，但两者必须复用同一个本地 billing identity，避免形成两套幂等主键。

## Goals / Non-Goals

### Goals

- 终态 usage 一旦被网关完整解析，进程崩溃后仍可按原始 payload 恢复并且只结算一次。
- 用户余额、API Key 配额、账号配额、账务幂等和对应 usage 明细不存在可观察的半提交状态。
- 已到达上游但无法取得终态 usage 的请求不会静默消失，并能区分“待恢复”“需要 provider/人工对账”和“已结算”。
- 用真实子进程终止和数据库故障注入证明恢复能力，而不仅是 mock 一个错误返回。
- 现有协议响应、价格/倍率、调度语义和默认部署方式保持不变。

### Non-Goals

- 不统一或删除两套 `RecordUsage` 计费计算核心；该工作必须另行证明语义一致。
- 不实现请求前额度预留，不替代 `harden-usage-billing-admission`。
- 不改变任何 provider 的 token 定义、长上下文、缓存、媒体或 Profit 口径。
- 不保证所有 provider 都能查询崩溃前未返回的终态 usage；不支持查询时只提供明确的待对账状态。
- 不在本 change 中新增 Prometheus/OTel、readiness 或通用优雅排水能力。

## Failure Windows

| Window | Current outcome | Required outcome |
|---|---|---|
| Upstream accepted → first/terminal usage | only in request/upstream state | persist attempt identity; unresolved exit remains auditable |
| Terminal usage parsed → worker starts | payload exists only in memory | persist versioned settlement event before async handoff |
| Billing transaction before commit | PostgreSQL rolls back | retry same event and billing identity safely |
| Billing commit → usage log commit | charged but detail can be missing | one transaction commits both or neither |
| Commit → worker acknowledgement | caller may not know commit result | retry observes idempotent applied state and acknowledges event |

## Decisions

### Decision 1: Use a PostgreSQL-backed settlement state machine

为每个本地 billing identity 保存一条可恢复记录，状态至少包含：

- `started`: 请求已经获准进入上游，保留 request/turn identity、API Key、账号、provider 和请求指纹；
- `ready`: 已持久化完整且版本化的 usage/计费输入快照，可以异步结算；
- `processing`: worker 已取得短租约；租约过期后允许其他实例重取；
- `settled`: 账务和 usage 明细已原子提交；
- `reconciliation_required`: 上游可能已产生成本，但终态 usage 无法恢复；
- `failed_retryable` / `failed_terminal`: 区分可重试基础设施故障与需要人工处理的输入/价格冲突。

状态记录必须由 PostgreSQL 持久化。Redis 和进程内队列只能作为唤醒/加速手段，不能成为唯一任务来源。

### Decision 2: Persist complete terminal payload before asynchronous billing

Relay/provider extractor 仍负责生成现有 token 语义；获得完整终态 usage 后，把结算所需的标量快照持久化为带 schema version 的 payload。持久化成功后才能把任务视为已交接给后台 worker。

不把 `ParsedRequest`、原始提示词、响应正文或凭据写入 journal。只保存计费与审计所需的 ID、模型、usage、倍率解析输入、endpoint/channel 归因和请求指纹，敏感字段遵循现有 usage log 数据边界。

### Decision 3: Atomically commit billing effects and usage evidence

扩展统一账务 repository 命令，使以下内容在同一 PostgreSQL 事务中提交：

- settlement/billing 幂等 claim；
- 用户余额或后续 reservation capture；
- API Key quota/rate-limit 数据库字段；
- API Key/Bedrock 账号 quota 字段及相关事务内 outbox；
- 对应 `usage_logs` 行；
- settlement event 的 `settled` 状态和提交时间。

缓存、低余额通知和展示型平台配额镜像仍可在事务后异步更新，因为它们可以从 PostgreSQL 真相源重新构建。事务提交后到缓存更新之间的短暂延迟不定义为账务丢失。

#### 与 admission reservation 的 identity/事务组合规则

- reservation 与 settlement 必须共用 handler 在进入上游前生成的本地 `billing_identity`；HTTP 请求与 WS turn 各自一个 identity，provider response ID 只是追踪字段。
- 数据库唯一性统一使用 `(billing_identity, api_key_id)`；reservation 不得再生成第二个 request ID，settlement 也不得用上游 ID 覆盖本地 identity。
- request fingerprint 用于证明同一 identity 仍对应同一请求；terminal payload 内的 billing fingerprint 则证明金额/usage 命令没有变化，两者不互相替代。
- 未来 admission 接入后，`capture(actual)` 必须与 billing dedup、usage log 和 settlement `settled` 位于同一 PostgreSQL 事务；不允许先 capture 后单独标记 settled。
- 只有尚未存在 ready terminal payload 的失败请求才可 release reservation。已有 ready payload 或已进入 processing 的任务必须由 settlement 事务 capture/结算，不得由 TTL 扫描器盲目 release。
- 本 change 不实现 `harden-usage-billing-admission` 尚未完成的 reserve/capture/release 表；当该 change 接入时，扩展现有原子 settlement 命令，不再建第二个结算事务。

### Decision 4: Do not guess incomplete streaming usage

请求进入上游后，如果进程退出、上游提前断流或始终没有 terminal usage：

- 有 provider 查询能力时，恢复器按稳定 provider request ID 查询并补全；
- 无查询能力或查询超过截止时间时，状态转为 `reconciliation_required`；
- 不用已观察到的 partial token 自动结算，也不写成零费用成功；
- 运维可按 provider、账号、时间和请求 ID 导出待对账项。

这是对“无法知道精确用量”的显式表达，不改变现有价格或扣费算法。

### Decision 5: Keep the current worker pool only as an accelerator

现有 worker pool 可继续用于低延迟唤醒，但 submit 失败、进程重启或任务超时后，数据库扫描器必须能够重新 claim `ready`/过期 `processing` 事件。默认 sync overflow 可以保留，但不再承担持久性保证。

### Decision 6: Prove behavior with differential and crash-injection tests first

实现生产切换前先建立测试床：

1. 使用确定性 fake upstream 和隔离 PostgreSQL/Redis，向旧、新实现发送相同 HTTP JSON、SSE 和 WS 请求；
2. 逐字段比较响应 schema、usage、金额、倍率、余额、API Key quota 和账号 quota；
3. 在 `terminal parsed`、`event persisted`、`billing effects executed before commit`、`commit before ack` 等边界终止真实子进程；
4. 重启后等待恢复器收敛，验证每个请求最终恰好一条 usage 明细、恰好一次扣费；
5. 对 terminal usage 永久缺失的场景验证其进入待对账状态，而不是被错误结算。

故障点通过测试专用 seam/子进程参数注入，不新增生产 YAML/env 配置键，不暴露管理或公网协议入口。

### Decision 7: Require an explicit database-backed group rollout scope

`usage_settlement_durability_mode` 只表示 `off/shadow/authoritative`，非 `off` 时还必须从数据库 setting `usage_settlement_durability_group_ids` 读取显式 scope。scope 只接受正整数 JSON 数组；`*` 仅供明确的隔离全量测试。缺失、空、非法或含非正整数时，active mode fail-closed 为 `off`。两个 setting 均只在实例启动时读取，不新增 YAML/env 键。

同一 group scope 必须同时约束：

- handler 是否同步执行 durable handoff，未选中 group 仍走现有 worker pool；
- upstream accepted 后是否写 `started`，terminal 后是否写 `ready`；
- terminal 结算是否跳过旧扣费路径；
- PostgreSQL worker 能否 claim 对应 group 的 ready/过期 processing 行。

这使测试 group 与旧路径 group 可在同一实例并存，且不得仅在 handler 做表面门控后让 worker 跨 group claim。

### Decision 8: Bind claim eligibility to the mode at attempt creation

每条 settlement attempt 必须持久化其创建时是否属于 authoritative 路径。shadow 创建的事件只用于差分审计，即使同一数据库后续把相同 group 切到 authoritative，也不得被 worker claim；authoritative 创建的事件才可进入 claim/retry/重启恢复。

该资格不能在 `shadow -> authoritative` 切换时批量升级，也不能由同 identity 的后续幂等 `StartAttempt` 改写。迁移对历史行使用 `false` 默认值，确保升级前或 shadow 阶段已有 journal 不会在新 worker 启动后被追溯扣费。worker 的 group scope 是第二层过滤，不能替代事件自身的 authoritative 资格。

## Alternatives Considered

- **只把 `RecordUsage` 改为同步执行**：缩短但不能消除 terminal usage 到数据库提交之间的进程崩溃窗口，也会把数据库尾延迟放回请求路径。
- **只在现有 worker pool 上增加 StopAndWait**：正常 SIGTERM 已有类似保护，无法覆盖 OOM、SIGKILL 和节点故障。
- **只依赖 `usage_billing_dedup`**：该表没有 token、金额和完整归因字段，不能恢复未执行或缺明细的结算。
- **复用 `billing_usage_entries`**：当前表依赖已存在的 `usage_log_id`，不能承载 usage log 之前的 durable handoff，且运行时代码未使用；可迁移或替换，但不能原样充当 journal。
- **每个 SSE usage patch 同步写数据库**：可以缩小 first-usage 窗口，但显著放大长流写压力，且 partial usage 不一定可安全计费。优先保存 attempt，再对有查询能力的 provider 恢复精确终态。

## Risks / Trade-offs

- 每个计费请求至少增加 attempt/ready 状态写入，可能增加 DB QPS 和尾延迟。通过批量 ready 持久化、连接池容量验证和 shadow 指标评估，不能以丢失持久性换性能。
- `started` 记录可能增长。必须定义终态、保留期和低成本归档，且不得在尚可 provider 对账时过早删除。
- 跨 change 修改 `usageBillingRepository` 容易与额度预留实现冲突。实现前冻结共同命令模型，分别测试 reservation identity 与 settlement event identity 的一一对应。
- 多实例 worker 可能重复 claim。数据库租约和幂等主键必须使重复执行无副作用。
- provider 查询能力不一致。规范只保证“可见且不静默丢失”，不虚构无法取得的精确 usage。

## Migration Plan

1. 先合入只包含 schema、repository 和 crash harness 的关闭状态实现，不接管生产结算。
2. shadow 写入 settlement attempt/event，同时仍由旧 `RecordUsage` 结算；对比事件 payload 与实际 usage log，不重复扣费。
3. 在 staging 对六个平台执行 HTTP/SSE/WS 差分和崩溃注入，验证积压、恢复和对账。
4. 仅对 `usage_settlement_durability_group_ids` 显式选中的测试分组启用 durable worker 为权威结算路径，未选中 group 保持旧路径；开关只放在数据库 setting，不新增 YAML/env 键。
5. 小流量灰度，持续核对 `settled event = usage_logs = billing dedup = balance delta`。
6. 全量稳定后移除旧的“handler 直接提交非持久化计费任务”入口；不在本 change 中删除两套费用计算核心。

## Rollback

- 在 shadow 阶段停用 journal 消费即可回到旧路径，已写事件不产生扣费。
- 权威切换阶段回滚时必须先停止 durable claim、排空 processing 租约，再恢复旧路径，避免双写；幂等 identity 保持不变。
- schema 为加法迁移，回滚二进制不要求立即删除表；待确认无回滚需求后再单独清理。

## Open Questions

- `started` 是否在所有平台转发前同步写入，还是首期只覆盖已有稳定 request/turn ID 的主协议面？
- 哪些 provider 支持按 request ID 查询精确终态 usage，查询认证和保留窗口分别是什么？
  - 2026-08-15 盘点结论：仅 OpenAI 官方 API Key stored Responses 与 xAI 官方 stored Responses 条件支持；其余当前生产形态均不能依赖精确历史查询。首期所有主入口都应记录 `started`，查询 adapter 只能作为条件增强，详见 `provider-recovery-inventory-2026-08-15.md`。
- usage log 的宽表 INSERT 是否直接并入账务 repository，还是使用同一 `sql.Tx` 下的窄 adapter？
- `reconciliation_required` 的保留期、告警阈值和人工处置权限由哪个运维面承载？
