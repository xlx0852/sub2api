# Change: 加固用量结算持久性

## Why

当前普通 HTTP、SSE 和 WS 请求在上游返回 usage 后，先把结算任务放入进程内 worker 队列，再执行数据库扣费；扣费事务提交后，`usage_logs` 又单独落库。进程若在这些边界被强制终止，可能出现“上游已产生成本但本地未扣费”，或“用户已扣费但缺少 usage 明细”的永久对账缺口。

已有 `harden-usage-billing-admission` change 解决的是请求前额度预留和并发透支，不提供终态 usage 的持久化队列，也不能恢复已经离开请求生命周期但尚未落库的结算。本 change 独立处理后结算持久性，不扩大已归档的 `refactor-architecture`。

## What Changes

- 为已到达上游的计费请求建立可恢复的 settlement attempt，并保存本地稳定计费身份和最小审计信息。
- 在获得终态 usage 后，先持久化完整、版本化的 settlement payload，再允许异步 worker 接管扣费。
- 由数据库驱动的 worker claim、重试和重启恢复替代“只有进程内队列知道该任务”的状态。
- 将账务幂等、余额/API Key/账号配额变更与对应 `usage_logs` 明细放入同一原子提交边界。
- 对已经到达上游但没有终态 usage 的请求保留 `reconciliation_required` 状态，不推测 token、不静默标记成功，并提供可审计的人工或 provider 对账入口。
- 建立旧实现/新实现差分与真实进程崩溃注入测试床，覆盖 SSE、WS、客户端断连和数据库提交边界。
- 保持协议 I/O、价格计算、倍率、调度、现有 YAML/env 键名及默认部署形态不变。

## Impact

- Affected specs: `usage-settlement-durability`
- Related changes: `harden-usage-billing-admission`（共享本地 billing identity 和 repository 热点，但目标与验收独立）
- Affected code: 网关 Relay/handler 结算交接、`UsageRecordWorkerPool`、`usageBillingRepository`、`usageLogRepository`、启动恢复与关闭排水
- Affected storage: 新增 settlement attempt/event 持久化结构；现有账务表只做兼容性迁移，不改金额口径
- Operational impact: 增加待结算积压、重试和待对账状态的运维可见性；上线必须先 shadow、再灰度
- **BREAKING**: 无对外协议破坏；内部结算时序和持久化边界会改变，因此必须经过差分和故障注入门禁
