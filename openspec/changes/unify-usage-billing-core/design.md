## Context

`GatewayService.RecordUsage` 与 `OpenAIGatewayService.RecordUsage` 已共享 `UsageBillingCommand`、`applyUsageBilling`、repository 原子提交、post-commit 副作用和 usage-log 持久化，但费用计算与日志组装仍重复。`app` 层禁止 import `service`，因此统一核心必须只依赖 domain/ports 或纯标量输入；service 只负责把 provider forward result 和现有实体快照映射进去。

基线提交为 `6373e92fac25ebc65897182fb6a8d8929ea54363`。实现必须在该干净 ref 上运行 durable settlement 差分测试床。

## Goals / Non-Goals

- Goals:
  - 一个费用计算入口、一个 usage-log 构造入口、一个结算 Record 输入。
  - 共享逻辑只有一份，平台差异以命名策略字段或回调表达。
  - 保持协议 I/O、token 桶、价格、倍率、扣费、Profit、配置与部署语义。
- Non-goals:
  - 不统一两套调度器。
  - 不改变 durable settlement 的 `off/shadow/authoritative` 或 legacy fallback。
  - 不把 provider 协议结构带入 `app/billing`。

## Decisions

### Decision 1: `app/billing` owns scalar contracts and pure orchestration

`billing.RecordInput` 承载统一 usage、请求/端点/渠道归因、可选媒体/WS/cyber 字段和 domain 实体快照。价格查询、账号能力和 group multiplier 通过窄接口/函数依赖注入，避免 `app -> service` 反向依赖。

### Decision 2: Preserve differences as named policy, not copied functions

统一 calculator 接受显式 policy：

- Gateway Gemini long context 固定阈值 `200000`、倍率 `2.0`；OpenAI 走账号开关。
- Gateway 缺价记录计算错误后以零费用继续落 usage；OpenAI 缺价记录结构化告警后以 token mode 零费用继续落 usage。
- OpenAI input/cache/cache-creation 是互斥桶；Gateway 保留 `ForceCacheBilling`。
- OpenAI WS 在 durable 未启用时保留上游 request ID 幂等规则。
- `CyberBlocked`、compact、service tier、video/image/WS 字段只在输入明确提供时写入。

禁止用默认零值暗示平台语义；每个差异都必须有命名测试。

### Decision 3: One usage-log builder consumes the calculated result

calculator 返回完整费用标量与规范化 usage；builder 在一次调用中生成 `domain.UsageLog`。平台专属字段是可选值，公共字段不再由两套函数分别赋值。

### Decision 4: Keep rollout persistence branches, remove core adapters

统一入口仍先执行 durable handoff；authoritative 成功后返回，其他模式继续既有 legacy commit。此次只合并计算与日志主体，不提前执行 `harden-usage-settlement-durability` 6.5 的旧路径删除。`settlement_adapters.go` 在所有调用者迁移后删除。

## Risks / Trade-offs

- 单一核心影响所有平台；任何桶或默认值错误都会扩散。以旧/新 worktree 差分和现有两套测试集作为硬门禁。
- 统一输入字段较多，但显式可选字段优于隐藏平台类型断言。
- service 到 app 的映射仍存在，但只做协议结果提取，不再包含费用或日志语义。

## Migration Plan

1. 固定 baseline 并补齐平台差异 characterization tests。
2. 引入统一输入/策略和纯 calculator，先让旧入口调用新核心。
3. 合并 log builder 与持久化 orchestration。
4. 迁移 handler/recovery 调用点，删除 adapter 和重复函数。
5. 运行针对性测试、三套差分、archtest、OpenSpec strict 与根构建。

## Rollback

本 change 独立提交；回滚该提交即可恢复两套费用计算主体。数据库 schema、协议、配置与 durable journal 均不变化。
