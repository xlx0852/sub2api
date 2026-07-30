## Context

现有订阅账本以 `account_subscription_cycles` 记录每次真实充值，账号抽屉用“周期收入 - 整期采购费”展示回本结果，全局利润则把周期费用按查询区间与周期的交集直线摊销。

上游封禁不是普通限流或暂时停用：账号不再产生收入，未退回的采购成本已经不可回收。通用 `status` / `schedulable` 开关能停止调度，但它们无法表达封禁时点、退款或损失发生时间，因此不能兼任财务账本。

## Goals / Non-Goals

- Goals:
  - 显式记录订阅周期因上游封禁而提前终止。
  - 结算与账号停止调度为一个原子操作。
  - 按实际收到的退款持续冲减亏损，保留发生日期和备注。
  - 账号回本数据与全局期间利润使用同一笔封禁/退款账本，且全局查询不出现 N+1。
  - 允许受控撤销误标，但不在撤销时自动恢复供给。
- Non-Goals:
  - 不根据 401/403、错误文本、连通性测试或通用 `disabled` 状态自动生成封禁结算。
  - 不把未到账的预计退款计入账本。
  - 不处理用户侧赔付、余额退回或销售收入冲正。
  - 不改变未封禁订阅周期和 API Key 按量账号的现有成本公式。

## Decisions

- Decision: 封禁必须由管理员在某个已记录的订阅周期上显式确认。
  - Rationale: 调度错误、token 到期、临时风控和真实封禁的财务后果不同，自动推断容易把可恢复账号误确认为损失。
- Decision: 新增 `account_subscription_terminations` 和 `account_subscription_refunds` 账本。前者保存周期、账号、封禁生效时间、原因、备注及可审计的撤销信息；后者保存实际到账时间、金额、币种、备注及可审计的冲正信息。
  - Rationale: 封禁是一次运营/财务终止事件，退款可以在之后分多次到账；将两者分开可以如实表达时间和金额，不需要覆盖原始结算。
- Decision: 一个周期同一时间最多只能有一条未撤销终止记录；退款总额不得超过周期采购费，金额为 0 的周期不允许新增正数退款。
  - Rationale: 防止重复结算和超额冲减造成负采购成本。
- Decision: 账号层面的封禁结果使用以下公式，金额统一按系统货币精度四舍五入：
  - `refund_total = SUM(未冲正且已到账的退款)`
  - `net_purchase_cost = max(0, period_fee - refund_total)`
  - `revenue_before_ban = SUM(actual_cost), created_at ∈ [cycle_start, min(banned_at, cycle_end))`
  - `recovered_amount = min(period_fee, revenue_before_ban + refund_total)`
  - `recovery_progress = period_fee == 0 ? 100 : recovered_amount / period_fee * 100`
  - `realized_profit = revenue_before_ban - net_purchase_cost`
  - `realized_loss = max(0, -realized_profit)`
  - Rationale: 已经实现的销售收入和实际退款都能回收采购本金；亏损应由原始账本派生，不再设置一个独立可编辑数字。
- Decision: 全局期间成本按事件时间确认。对费用 `F`、周期 `[S,E)`、封禁时间 `B` 的周期：
  - `[S,B)` 内仍按 `F / period_days` 直线摊销。
  - `B` 时点确认 `F - gross_amortized(S,B)` 的剩余成本。
  - `B` 之后不再继续日摊销。
  - 每笔退款在 `received_at` 确认 `-refund_amount` 成本调整。
  - 因此整个生命周期的累计成本恒等于 `F - refund_total`，且封禁前的已关闭期间不会被静默重写。
  - Rationale: 封禁使未摊销成本在当日变成已发生损失；退款只能在真实到账时冲减，才能保持期间利润可解释。
- Decision: 终止记录、账号 `status=disabled`、账号 `schedulable=false`、共享凭据影子账号停用和 scheduler outbox 事件在同一数据库事务中写入；事务提交后再同步快照和失效利润缓存。
  - Rationale: 不允许出现“财务已确认封禁但账号仍在接单”或“账号已停但账本没写入”的部分成功状态。
- Decision: 结算撤销仅用于纠正误标，撤销后该周期回到普通摊销口径，已实际到账且未冲正的退款仍作为负成本保留。账号不会自动改回 `active` 或 `schedulable=true`。
  - Rationale: 财务纠错不应隐式扩大为恢复上游流量的操作。
- Decision: 成本弹窗中的周期行提供“封禁结算”，确认层要求封禁时间、可选初始已到账退款和备注，并明示“会立即停止账号调度”。结算后周期行改为红色终止摘要，且可继续追加实际退款。
  - Rationale: 操作入口与原始采购周期在同一界面，可以降低结算错账号或错周期的风险。
- Decision: 全局利润一次批量加载查询范围需要的终止与退款记录，不在账号或周期循环中查数据库。
  - Rationale: 新账本不应破坏现有利润 overview 的 O(1) 查询预算。

## API Shape

- `POST /api/v1/admin/profit/cycles/:id/termination-preview`
  - body: `effective_at`, optional `initial_refund_amount`
  - result: 基于封禁前真实收入的派生亏损摘要，不写账本或修改调度
- `POST /api/v1/admin/profit/cycles/:id/termination`
  - body: `effective_at`, `reason`, `notes`, optional `initial_refund_amount`, optional `initial_refund_received_at`
  - result: 终止记录、派生亏损摘要和已停用账号 ID
- `POST /api/v1/admin/profit/terminations/:id/refunds`
  - body: `amount`, `received_at`, `notes`
  - result: 退款记录和重算后亏损摘要
- `POST /api/v1/admin/profit/refunds/:id/void`
  - body: `reason`
  - result: 退款冲正和重算后亏损摘要
- `POST /api/v1/admin/profit/terminations/:id/reverse`
  - body: `reason`
  - result: 已撤销的终止记录；账号仍保持停用
- 现有周期列表响应增加 `termination`、`refunds` 和派生 `loss_summary`；无结算周期的旧字段保持不变。

## Risks / Trade-offs

- 封禁当日会出现一笔较大的成本确认，日利润曲线可能突然下降。
  - Mitigation: 趋势和账号明细标识该笔为“封禁损失确认”，而不是当日用量成本。
- 管理员可能误把临时风控标记为封禁。
  - Mitigation: 禁止自动结算，在确认界面展示账号、周期、估算亏损与停止调度影响，并保留受审计撤销。
- 财务事件与账号状态涉及多表原子性。
  - Mitigation: 在仓储层使用单个 SQL 事务、行锁和幂等约束，事务失败不进行缓存同步或成功响应。
- 退款到账晚于封禁，早期查询看到的亏损会高于最终结果。
  - Mitigation: 明确区分“已到账退款”和“已确认亏损”，每笔退款按到账日调整。

## Migration Plan

1. 新增终止与退款账本表、唯一/外键/金额约束及按周期与时间的索引；不回填或猜测任何历史封禁。
2. 发布批量加载、期间成本计算和账号派生亏损摘要，默认对无终止记录的周期保持原公式。
3. 发布管理 API 和前端确认界面，验证封禁事务后账号及影子账号已从调度快照移除。
4. 用一个测试账号验证无退款、首次部分退款、延迟追加退款、退款冲正与结算撤销。

## Open Questions

- 当账号已经超额回本时，当前方案保留正利润并将亏损显示为 0；不另外生成“机会损失”。
