## Context

`scheduling.Engine` 已统一入口，`CandidatePolicy`、候选查询、priority/LRU 与粘性纯决策也已下沉。但生产选号仍分叉：

- Gateway：分层硬过滤、sticky 优先、priority/LRU 和先到先得，外加 session/RPM/window-cost/wait/mixed-platform 副作用。
- OpenAI：previous-response/session/load-balance 三层，运行时 error/TTFT/compact 统计、归一化加权评分、TopK 加权探索与 sticky escape。

评分制转正会改变最终账号分布，不能用“架构清理”掩盖行为变化。现有 `harden-openai-pool-outcome-sticky-stream` 仍有 sticky 容量、stall usage、observe/group rollout 和运维说明未收口，本 change 必须消费其稳定接口而不是抢占实现所有权。

## Goals / Non-Goals

### Goals

1. 形成单一共享评分与选择内核，同时保持平台硬门槛显式、可排序、可诊断。
2. 为 Gateway 采集评分所需的有界运行时反馈，不把 OpenAI service 状态直接搬进 `app/scheduling`。
3. 用同输入差分、shadow、按 group canary 和 kill switch 控制选号语义变化。
4. 证据充分后删除两套旧调度主体，使 `scheduling.Engine` 不再只是门面。

### Non-Goals

- 统一平台协议、计费、并发服务、等待队列或 quota provider。
- 让软评分绕过不可调度、分组、能力、凭证类、RPM、quota/cooldown 等硬门槛。
- 在本 change 中修复 fingerprint、WSv2 originator 或其他 dirty WIP 测试。
- 立即把评分制默认开启到所有平台/分组。

## Decisions

### Decision 1: 统一选择流水线

共享内核按固定阶段运行：

```text
Resolve request context
  -> Enumerate candidates
  -> CandidatePolicy hard gates
  -> Affinity decision
  -> Build immutable snapshots
  -> Normalize and score
  -> TopK weighted order
  -> platform acquire/wait adapter
  -> outcome feedback
```

`app/scheduling` 只接收不可变快照与纯值，不读取 repository、Redis、service 或平台协议。枚举、粘性存储、并发槽和 wait plan 仍通过 ports/adapter 提供。

### Decision 2: 硬门槛优先于任何分数

最少包含以下 policy 类别：excluded、platform/group、schedulable/model、credential class、transport/capability、quota pause/cooldown、RPM/window-cost、shadow-parent/mixed-platform。平台可声明额外 policy，但必须有稳定名称并按顺序返回首个拒绝原因。

shadow 或 enforce 若尝试选择被任一硬 policy 拒绝的账号，视为安全违例并立即回退旧实现；该违例阻断灰度扩容。

### Decision 3: 平台无关候选快照

共享快照包含：

- account identity、priority、credential class 与 capability 标签
- load rate、active/waiting、quota/window headroom、reset horizon
- error-rate EWMA、TTFT EWMA、compact error/latency/penalty
- previous-response/session affinity 命中与 sticky escape 信号
- 每个信号的 available 位，避免把“未知”误当零值优势

Gateway 与 OpenAI 分别构造快照；共享内核不得反向依赖两种 service 类型。

### Decision 4: 先精确迁移 OpenAI 公式

第一版共享 scorer 必须通过 characterization，逐项复现当前 OpenAI 归一化、权重、tie-break、TopK 和加权顺序。测试随机源/时钟可注入；生产无会话锚点时仍保留现有探索熵。

Gateway 的缺失信号以 unavailable 表达。Gateway 开启 enforce 前必须完成代表窗口的统计预热；预热不足时保持旧规则制，不以全零快照强行评分。

### Decision 5: Affinity 是显式阶段

previous-response、session sticky、generation/class 校验和 sticky escape 先形成 `AffinityDecision`。默认 Gateway hard-sticky 与现有 OpenAI 三层顺序在 `off`/`shadow` 不变；只有 enforce scope 才可按已批准规则让 sticky escape 或 weighted sticky 改变选择。

本 change 不重新定义 `harden-openai-pool-outcome-sticky-stream` 已拥有的 re-eval、lowest-usage、credential generation 和 outcome class。

### Decision 6: 有界运行时统计与反馈

共享 `StatsStore` 以 account ID 为键，保存 EWMA/计数和最后更新时间，具备 TTL/容量上限及账号删除清理。平台 adapter 把既有上游 outcome、首 token、compact、并发/等待和 quota 快照映射为统一反馈。

统计只影响已启用的评分维度；过期或样本不足必须标 unavailable。不得记录提示词、响应正文、token、session hash 或凭据。

### Decision 7: 三态 rollout

- `off`：只运行旧实现，默认值。
- `shadow`：旧实现产生唯一真实选择；新内核只计算 counterfactual，不获取槽位、不写 sticky、不入等待队列。
- `enforce`：在明确 platform/group scope 内由新内核产生选择；内部错误可无副作用回退旧实现并计数。

rollout 状态存入现有数据库 settings/control plane，不新增 YAML/env 键。每次扩容必须保留一键 `off`，不要求重启。

### Decision 8: 差分与准入标准

同输入测试至少比较：原始候选、每个 policy 的拒绝集合、有效候选、归一化信号、score、TopK、最终账号、wait plan、sticky/escape reason。

shadow 允许评分目标导致的最终账号差异，但所有差异必须可归因；以下任一项为硬失败：

- 新实现选择旧硬门槛会拒绝的账号
- credential class、capability、quota/cooldown、RPM/window-cost 或 mixed-platform 边界越界
- account acquire/release、session 注册或 wait plan 出现双副作用
- error rate、TTFT、failover 或 quota 分布超过基线批准阈值

阈值必须从至少一个正常窗口和一个代表峰值窗口确定，不在无生产证据时拍定。

### Decision 9: 删除旧主体的最终门禁

只有所有目标平台均完成：本地差分、staging 真实 HTTP/SSE/WS、shadow 两类窗口、按 group canary、故障注入、计费不变证明、成对回滚，且 kill switch 未被触发，才允许删除旧 Gateway/OpenAI 调度主体。

删除必须独立任务完成，并保留纯 scorer/adapter 级回放夹具作为长期回归；不能因入口已经统一而提前宣称终态完成。

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Gateway 统计冷启动把未知账号误判为优/劣 | available 位 + 预热门槛 + 未预热保持旧规则制 |
| TopK 探索破坏 sticky 或 prompt cache | affinity 独立阶段、按 group enforce、监测 account switch/cache 指标 |
| 平台 policy 顺序变化导致边界越权 | characterization 锁定首拒绝原因和有效候选集合 |
| shadow 产生重复并发/等待副作用 | shadow scorer 只接收快照，禁止调用 acquire/sticky/wait ports |
| 与 outcome/sticky change 冲突 | 明确符号所有权；依赖 change 未收口部分先完成或提供稳定接口 |
| 回滚后统计状态污染旧实现 | 旧实现不读取共享 scorer 状态；`off` 即时停止其选号影响 |

## Migration Plan

1. 采集 staging 和获授权的生产只读窗口，固定候选/决策回放夹具及 SLO 基线。
2. 在 `app/scheduling` 实现纯 scorer、TopK、快照和 policy 诊断，先对拍 OpenAI 现实现。
3. 接入 OpenAI shadow，再补 Gateway 统计和 policy adapter；默认仍 `off`。
4. staging 运行 shadow 与故障注入，证明无副作用、无硬门槛越界。
5. 生产按单 group `shadow -> enforce`，逐步扩大 platform/group scope；异常立即 `off`。
6. 完成全平台门禁和成对回滚后，独立删除旧主体与过渡适配器。

## Open Questions

- 正常/峰值生产窗口及 SLO 阈值需要只读证据后填定；在获得美国生产只读授权前保持未决。
- Gateway 哪些 request class 首批进入 enforce，应根据账号数量、mixed Antigravity 使用和统计预热选择，不在 proposal 阶段假定。
