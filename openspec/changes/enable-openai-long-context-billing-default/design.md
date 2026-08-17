## Context

`Account.IsOpenAILongContextBillingEnabled()` 当前只在 `extra` 中读取布尔 `true`，缺失和无效值都返回 false。OpenAI usage extractor 把该结果作为显式 policy 传入统一计费核心，因此模型目录虽然已有阈值和倍率，生产 OpenAI 请求仍可能不应用。

## Goals / Non-Goals

- Goals:
  - 缺省按官方长上下文价格计费。
  - 保留显式 `false` 覆盖能力。
  - 不改变阈值、token 桶或整单倍率算法。
- Non-Goals:
  - 不追溯补扣历史请求。
  - 不改变非 OpenAI provider 或自定义区间价格。

## Decisions

### Decision 1: 在账号语义层将缺省值改为 true

OpenAI 账号 `extra` 缺失该键或值不是 bool 时返回 true；显式 bool 原样返回。非 OpenAI 账号仍返回 false。这样 HTTP/SSE/WS 和 shadow credential-account 解析继续共用同一策略。

### Decision 2: 保留模型目录作为最终准入

账号默认启用不代表所有模型都加价。计费核心仍要求模型存在正数 threshold 和 multiplier，并且实际输入 token 严格大于 threshold。

### Decision 3: 不持久化默认值

不批量更新账号 extra。默认语义由代码解释，避免无必要数据库写入；显式 false 仍可通过现有账号 extra 管理方式保存。

## Risks / Trade-offs

- 长上下文请求扣费会增加。缓解：这是对齐官方价格的预期变化，并保留显式 false。
- 未知/损坏 extra 会从旧 false 变为 true。缓解：只有模型目录定义长上下文规则且 usage 越过阈值时才生效。
- 渠道自定义区间价格可能已含分层。缓解：现有计费核心对 interval pricing 不重复应用长上下文倍率，保持不变。

## Migration Plan

1. 先改 characterization tests，锁定缺省 true、显式 false 和非 OpenAI false。
2. 修改账号默认语义并运行 OpenAI HTTP/SSE/WS usage 回归。
3. staging 以 >272K 合成 usage 验证费用，确认普通请求不变。
4. backend-only 蓝绿发布；回滚旧二进制即可恢复旧默认。
