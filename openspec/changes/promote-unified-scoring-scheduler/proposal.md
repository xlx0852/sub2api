# Change: 将评分制调度转正为统一调度引擎

## Why

当前所有平台已经从 `scheduling.Engine` 进入，但 Gateway 规则制与 OpenAI 评分制仍是 Strategy 后面的两套活实现。继续并存会让粘性、硬门槛、运行时反馈和候选排序持续漂移；直接删除任一主体又会改变选号、冷却和失败恢复语义。

本 change 选择 Change 3 的“转正”路线：把已经在线运行的 OpenAI 评分、TopK 探索和运行时统计抽成 `app/scheduling` 的共享能力，再让 Gateway 以显式 `CandidatePolicy` 提供平台硬门槛。迁移必须先 shadow 对拍、再按 group 灰度，最后才允许删除旧主体。

## What Changes

- 在 `app/scheduling` 定义平台无关的候选快照、可用信号、评分权重、TopK 排序/加权选择和可注入随机源。
- 所有候选先通过按声明顺序执行的 `CandidatePolicy`；评分只能在硬门槛通过的集合中发生。
- OpenAI 将现有评分公式、sticky escape、TopK 和运行时反馈迁入共享内核，第一阶段必须对现有高级调度器保持逐输入一致。
- Gateway 补齐有界的 error/TTFT/load/wait/quota-window 运行时快照；Claude fallback、session 注册、RPM/window-cost、wait plan、mixed Antigravity 等规则保留为显式 policy/adapter。
- 增加数据库控制的 `off` / `shadow` / `enforce` rollout 和 platform/group scope；默认 `off`，不新增或改名 YAML/env 键。
- shadow 同时计算旧/新决策但只允许旧实现产生并发、sticky、等待队列等副作用；输出候选门槛、排序和选择差异的无凭据证据。
- 各平台通过正常窗口、峰值窗口、staging 差分、按 group 灰度和成对回滚后，才删除 Gateway/OpenAI 两套旧调度主体与过渡适配器。

## Impact

- Affected specs:
  - `architecture-governance`
  - `account-scheduling-policy`（新）
- Affected code（批准后实施）:
  - `backend/internal/app/scheduling`
  - `backend/internal/service/gateway_scheduling.go`
  - `backend/internal/service/openai_account_scheduler.go`
  - `backend/internal/service/account_selector_adapters.go`
  - 调度反馈、设置、ops 结构化出口及相关测试/差分工具
- **Behavior change**: 仅 `enforce` scope 内允许评分/TopK/sticky escape 改变 Gateway 或 OpenAI 的最终选号；`off` 保持旧实现，`shadow` 不改变选择。
- Preserved behavior: 对外协议 I/O、计费/倍率、并发额度、quota/cooldown 硬排除、YAML/env 键名和默认部署/升级形态不变。
- Related changes:
  - `refactor-scheduling-equivalent-primitives` 提供共享基础 policy/排序/粘性决策。
  - `harden-openai-pool-outcome-sticky-stream` 继续拥有 outcome class、credential generation、sticky re-eval、lowest-usage 与 stall 结算语义；其未收口任务不得被本 change 重写或顺便修复。
- Production evidence: 美国生产窗口只允许只读采样，且仍需用户单独明确授权；提案本身不授权访问生产。

## Non-Goals

- 不在 proposal 审批前修改生产调度代码。
- 不机械合并两套旧主体，不在 shadow/灰度证据完成前删除回滚路径。
- 不改变价格、扣费、协议格式、账号凭据、YAML/env 配置或客户端会话标识。
- 不给 `service` 类型别名增加方法，不运行 Wire 作为本 change 的通过条件。
