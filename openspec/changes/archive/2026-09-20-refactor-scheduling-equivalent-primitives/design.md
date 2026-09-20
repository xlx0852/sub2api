## Context

`scheduling.Engine` 已是统一入口，但 Gateway/OpenAI Strategy 仍调用各自的活调度主体。当前审计确认：

- `checkChannelPricingRestriction` 两份实现逐字等价。
- `listSchedulableAccounts` 只有 snapshot/repository 查询骨架等价；Gateway 的 mixed scheduling 与 OpenAI 的兼容平台规则不等价。
- priority/LRU 排序和 shuffle 已由两侧共用 Gateway 文件中的唯一实现，本 change 只修正归属。
- Gateway Layer 1.5、OpenAI `tryStickySessionHit`、高级调度器 `selectBySessionHash` 具有显著语义差异，不是可整段合并的副本。

## Goals / Non-Goals

- Goals:
  - 只抽取可用 characterization/differential tests 证明的等价原语。
  - 所有平台差异保持显式，候选集、顺序、粘性命中和排除结果不变。
  - 为后续调度哲学决策建立可复用差分测试入口。
- Non-goals:
  - 不合并或删除 `gateway_scheduling.go`、`openai_gateway_scheduling.go`、`openai_account_scheduler.go` 主体。
  - 不改评分公式、TopK、sticky escape、门槛顺序、冷却、配额暂停、失败惩罚或凭证类别。
  - 不新增 YAML/env/settings 键。

## Decisions

### Decision 1: Extract pure decisions, keep stateful effects in Strategy

`app/scheduling` 原语只接收标量/候选元数据并返回决策。Redis sticky 读写、session 注册、epoch 检查、DB refresh、runtime stats 与 concurrency acquire 仍由原 service 路径执行。

### Decision 2: Shared account listing is a query skeleton plus named policy

共享骨架只处理 snapshot 优先、grouped/ungrouped repository 分支和错误返回。Gateway mixed platforms/filter 与 OpenAI platform normalization 由调用方显式准备 policy，不用默认零值猜测。

### Decision 3: Existing randomization boundaries are contractual

共享 shuffle 仍只在完全相同的 sort-key run 内随机化。`preferOAuth` 的分区优先级、`accountWithLoad` 的 load-rate 分组和 nil `LastUsedAt` 语义都保持。

### Decision 4: Sticky extraction stops before policy divergence

公共原语可以决定绑定是否存在、是否在排除集、是否应清理、预取候选是否匹配；不得决定 Gateway session limit 或 OpenAI quota re-eval/escape。

## Risks / Trade-offs

- 公共帮助器影响所有平台选号。每个迁移必须先保留旧函数作对拍 oracle，确认同输入输出一致后才删旧实现。
- 抽象可能只减少少量代码，但明确停在语义分叉前比过度合并更重要。
- 与 `harden-openai-pool-outcome-sticky-stream` 有文件重叠。本 change 不修改其设置、指标或算法；如果合并时两者代码不一致，以其当前行为为 oracle 调整调用。

## Migration Plan

1. 固定 Gateway/OpenAI 调度 characterization 和公共原语差分测试。
2. 按 channel restriction → list query → sort/shuffle → sticky decision 的顺序小步迁移。
3. 每步跑 `app/scheduling` 与两套 service scheduler 针对性测试。
4. 运行 archtest、OpenSpec strict、根构建和调度差分用例后独立提交。

## Rollback

本 change 作为单独提交回滚；无 schema、配置或外部协议迁移。
