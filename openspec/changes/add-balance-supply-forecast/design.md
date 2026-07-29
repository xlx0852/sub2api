## Context

用户余额表示尚未消费的客户储值，而账号供给分为两类：订阅账号的固定周期产能，以及 API Key 账号的按量采购成本。二者不能使用同一个“余额 ÷ 账号价格”公式。项目已有 `users.balance`、`usage_logs.actual_cost`、`usage_logs.subscription_id`、账号认证类型、平台、分组关联、可调度状态和历史账号成本快照，可以在不新增财务事实表的前提下生成经营预测。

## Goals / Non-Goals

- Goals:
  - 量化当前可消费储值与按当前速度的余额支撑天数。
  - 按平台预测订阅账号数量缺口和 API Key 上游采购预算。
  - 所有数字可追溯到明确的时间窗口、样本量、安全余量和公式。
  - 预测加载不影响现有利润页首屏性能。
- Non-Goals:
  - 不把预测值写回用户余额、账号配置或订阅周期账本。
  - 不宣称预测账号数等于上游官方配额。
  - 不为按量 API Key 供给根据金额伪造账号数量。
  - 首期不做机器学习模型、季节性、客户流失或充值概率预测。

## Decisions

- Decision: “待履约储值”取未删除、正常状态、非管理员用户的 `MAX(balance, 0)` 之和；`frozen_balance` 单独展示且不计入可消费需求。
  - Rationale: 负余额不是供给需求，冻结余额在解冻前不可消费，管理员测试账户不应扭曲经营预测。
- Decision: 储值消耗只统计 `usage_logs.subscription_id IS NULL AND actual_cost > 0` 的余额计费记录。
  - Rationale: 有效订阅内的请求不扣减用户余额，若纳入会夸大储值消耗。
- Decision: 基础日需求为 `max(最近7天日均余额实扣, 最近30天日均余额实扣)`；规划日需求再乘以 `1 + safety_margin`。
  - Rationale: 容量规划宁可对近期增长保守一些，不应被 30 天长均值压低。同时展示 7 天和 30 天原值，便于管理员判断。
- Decision: `projected_consumption = min(spendable_balance, base_daily_demand × horizon_days)`，`runway_days = spendable_balance / base_daily_demand`；供给规划使用加安全余量后的日需求。
  - Rationale: 预计消耗不得超过当前可消费余额，而容量准备需要额外余量。
- Decision: 平台需求占比来自最近 30 天余额实扣收入，通过 `usage_logs.account_id -> accounts.platform` 归属。无历史时不输出平台账号数。
  - Rationale: 用户余额本身不绑定平台，只能用实际消费结构分配；无样本时强行均分会制造伪精确。
- Decision: 订阅账号的单号日承载量取最近 30 天“账号-活跃日”客户实扣收入的 P75，仅统计 OAuth / Setup Token 且存在用量的样本。`required_accounts = ceil(platform_planning_daily_demand / p75_account_daily_revenue)`。
  - Rationale: P75 比平均值更能抑制低活跃日导致的账号需求虚高，同时不使用极端最大值。该值仍是实现产能，不是官方上限。
- Decision: 当前订阅供给按平台去重统计认证类型为 OAuth / Setup Token 且符合生产调度过滤条件的账号。
  - Rationale: 同一账号可绑定多个分组，按分组相加会重复计数；过期、暂停、限流和不可调度账号不是当前供给。
- Decision: API Key 采购预算使用同平台历史 `metered_cost / revenue` 比率乘以按量部分的预计客户消耗。
  - Rationale: API Key 成本来自上游采购折扣快照，金额可估算；但不存在通用的固定“单账号产能”。
- Decision: 预测 API 使用 15 分钟有界快照，缓存键包含规划期、安全余量和时区；支持手动刷新，同键并发缺失合并回源。
  - Rationale: 供给规划不需要逐请求实时，且聚合用户余额与 30 天用量需要避免频繁扫描。

## Risks / Trade-offs

- 用户可能不会用完当前余额，储值总额不等于某个固定日期必然发生的需求。
  - Mitigation: 同时展示余额总额、速度法预测和可支撑天数，不将全部余额强制塞进规划期。
- 历史实现承载量可能被当时需求不足压低，从而高估需要账号数。
  - Mitigation: 使用 P75、展示样本数和置信度，并明确标注“基于历史实现产能”。
- 平台消费结构可能随新模型、价格或客户变化。
  - Mitigation: 提供规划期和安全余量调整，快照显示生成时间，后续可在不改 API 主结构的前提下增加趋势算法。
- 平台内同时存在订阅与 API Key 路由时，简单归一会掩盖两种成本。
  - Mitigation: 根据历史账号认证类型占比拆分订阅与按量需求，分别输出账号数与采购预算。

## Migration Plan

1. 新增预测仓储聚合、纯函数计算器、服务和管理端 API，不修改现有财务数据。
2. 增加 15 分钟快照与查询预算测试，在生产近似数据上执行只读 EXPLAIN ANALYZE。
3. 在利润分析中增加延迟加载页签，先展示公式、置信度和明细，不自动触发采购操作。
4. 部署后对照用户余额总额、最近 7/30 天实扣和当前可调度账号清单做只读核验。

## Open Questions

- 默认规划期按 30 天、安全余量按 20% 实现；如经营上有固定的采购周期，可在实施前调整默认值。
- 首期是否需要把“用户未使用余额分布”拆为大额用户 Top N；默认仅展示汇总与分布区间，避免页面过度拥挤。
