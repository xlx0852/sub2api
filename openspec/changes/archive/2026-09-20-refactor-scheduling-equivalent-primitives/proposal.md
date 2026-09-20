# Change: 收敛调度行为等价公共原语

## Why

Gateway 与 OpenAI 已经统一从 `scheduling.Engine` 进入，但 Strategy 后仍重复维护渠道定价限制、基础候选查询和粘性候选判定骨架。这些重复增加漂移风险，但两套调度主体的门槛和选号哲学仍未对齐，不能在本 change 删除或合并。

## What Changes

- 在 `app/scheduling` 提供不依赖 provider/service 的公共原语，用于渠道计费模型限制、等值分组 shuffle 和粘性候选结果判定。
- Gateway/OpenAI 的基础候选列表查询共用一份骨架，混合平台、privacy/credential、runtime block 等差异仍由显式平台策略执行。
- 已被 Gateway/OpenAI 共用的 priority/LRU 排序与 shuffle 从 service 文件移到 `app/scheduling`，不改比较键或随机分组边界。
- 粘性链只抽取完全等价的“绑定存在/排除/清理/候选命中”决策骨架；`checkAndRegisterSession`、sticky epoch、credential class、quota re-eval/escape、transport 兼容仍留在原 Strategy。
- 先为每个抽取原语建立新旧差分/characterization 测试，任何选号或候选集差异都阻断合并。

## Impact

- Affected specs: `architecture-governance`
- Affected code: `backend/internal/app/scheduling`，`backend/internal/service/gateway_*scheduling*`，`openai_*scheduling*`，渠道限制和调度测试
- Related active change: `harden-openai-pool-outcome-sticky-stream`；本 change 不改其 outcome class、sticky re-eval、lowest-usage 或 failover 语义
- **BREAKING**: 无对外协议、配置、选号或调度故障处置语义变化
- Non-goal: 不删除两套调度器主体，不决定评分制调度器的转正/废弃/并存命运
