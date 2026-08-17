# Change: 默认启用 OpenAI 长上下文计费

## Why

OpenAI 官方对支持 1.05M 上下文的 GPT 模型规定：输入超过 272K 后，整次请求输入按 2 倍、输出按 1.5 倍计价。项目已经实现该公式，但 OpenAI 账号缺少 `openai_long_context_billing_enabled` 时默认关闭，导致未显式配置的长上下文请求少计成本。

## What Changes

- **BREAKING**：OpenAI 账号未设置长上下文开关时，默认启用模型目录声明的长上下文计费规则。
- 显式配置 `openai_long_context_billing_enabled=false` 时继续关闭，作为人工覆盖能力。
- 非 OpenAI 账号保持关闭；没有长上下文定价元数据或未超过阈值的请求费用不变。
- 保持 `input + cache_creation + cache_read > threshold` 的触发口径，以及整次输入/缓存 × 输入倍率、整次输出 × 输出倍率的现有算法。
- 更新账号语义和 OpenAI RecordUsage 回归，覆盖缺省启用、显式关闭、显式开启及阈值边界。

## Impact

- Affected specs:
  - `usage-billing-core`
- Affected code:
  - `backend/internal/domain/account_entity.go`
  - `backend/internal/service/openai_gateway_usage.go`
  - `backend/internal/service/openai_gateway_record_usage_test.go`
  - 长上下文定价/账号实体相关测试
- Operational impact:
  - 现有 OpenAI 账号若未写该字段，部署后超过模型阈值的请求将自动按官方倍率扣费。
  - 已显式写 `false` 的账号不改变。
  - 无数据库迁移，rollback 恢复旧二进制即可恢复旧缺省语义。

## Non-Goals

- 不修改 272K 阈值、2.0/1.5 倍率或 1.05M 上下文元数据。
- 不新增“仅超出部分加价”算法。
- 不修改基础模型价格、service tier、分组倍率或渠道自定义区间价格。
- 不自动重写账号 `extra`，也不追溯重算历史 usage。
