# Change: 合并 Gateway/OpenAI usage 结算核心

## Why

Gateway 与 OpenAI 的 `RecordUsage` 输入和持久化下半场已经收敛，但费用计算、usage-log 构造和入口类型仍各维护一份。重复实现会让价格、倍率、缓存桶和新增 usage 字段逐步漂移，也使 durable settlement 必须通过适配器回到两套主体。

## What Changes

- 在 `internal/app/billing` 建立单一、纯标量的 `RecordInput`、费用计算结果、usage-log 输入和平台策略入口。
- 合并 token/image/video、channel pricing、group multiplier 和 usage normalization 的重复计算逻辑。
- 合并 Gateway/OpenAI usage-log 构造，OpenAI 独有字段作为显式可选字段。
- 将两套 `RecordUsage` 降为薄入口或删除，所有 handler、恢复器和管道调用统一结算核心。
- 删除 `settlement_adapters.go`，不再把统一输入映射回两套结算主体。
- 显式保留 Gemini long-context、OpenAI pricing-missing、WS identity、缓存桶、`ForceCacheBilling` 与 `CyberBlocked` 等既有差异。

## Impact

- Affected specs: `usage-billing-core`
- Related change: `harden-usage-settlement-durability`（复用其差分与崩溃测试床，不改变其 rollout gate）
- Affected code: `backend/internal/app/billing`, `backend/internal/service` 的两套 usage 费用/日志逻辑及其 handler/recovery 调用点
- **BREAKING**: 无对外协议破坏；内部入口与类型会合并
- Non-goals: 不合并调度器、不改变价格/倍率/扣费/usage 字段语义、不删除 durable/legacy rollout 双路径
