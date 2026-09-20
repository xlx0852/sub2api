## Context

旧单位下，支付金额被放大为余额，用户实际扣费也按同样的放大单位记录。本次变更是记账面值归一，不是降价、涨价或改变上游成本。

## Goals / Non-Goals

### Goals

- 新用户余额与充值金额按 `1:1` 表达。
- 迁移前后普通用户的可调用量不变；两个历史 `1:5` 充值账户按运营方确认的实际充值倍率恢复充值本金口径。
- 每个模型、分组和媒体类能力的相对售价不变。
- 平台同等实付金额可承载的上游成本和利润不变。
- 用户在余额、API Key 限额、用量记录和邀请返利页面看到一致的新单位。

### Non-Goals

- 不改变模型官方定价或 catalog/pricing JSON。
- 不改变支付通道币种、支付手续费率、最低/最高实付限制。
- 不改变账号侧成本统计倍率和上游账号配额。
- 不在本次变更中重新设计历史退款业务。

## Core Invariant

设全站客户计费放大因子 `S = 10`，特殊充值因子 `A = 5`：

```text
normal_migrated_balance = old_balance / S
special_recharge_delta = completed_old_recharge * (1 / A - 1 / S)
new_balance = normal_migrated_balance + special_recharge_delta
new_customer_charge = old_customer_charge / S
new_recharge_multiplier = old_effective_recharge_multiplier / S = 1
```

普通用户没有 `special_recharge_delta`，可调用量保持不变：

```text
old_balance / old_customer_charge
  = (old_balance / S) / (old_customer_charge / S)
  = new_balance / new_customer_charge
```

特殊账户只对历史充值事件使用 `A = 5`。不能把包含历史消耗后的净余额整体除以 `5`，否则会错误放大基础赠送与消耗；费率和用量字段仍使用 `S = 10`。

## Data Classification

### Divide by 10

| Domain | Fields |
| --- | --- |
| User funds | 普通用户字段除以 `10`；特殊账户余额先按 `÷10`，再增加已完成充值的 `1/10` 差额，`total_recharged` 按 `÷5` |
| Default grants | `default_balance` and every `auth_source_default_*_balance` setting |
| API key quota | `api_keys.quota`, `api_keys.quota_used` |
| Platform quota | all limit and usage fields in `user_platform_quotas` |
| Customer billing rate | `groups.rate_multiplier`; `user_group_rate_multipliers.rate_multiplier` |
| Independent media rate | `groups.image_rate_multiplier` only when `image_rate_independent=true`; `groups.video_rate_multiplier` only when `video_rate_independent=true` |
| Subscription quota | non-zero group daily/weekly/monthly limits and corresponding `user_subscriptions.*_usage_usd` |
| Customer usage history | `usage_logs.actual_cost`, `usage_logs.rate_multiplier`; dashboard hourly/daily `actual_cost` |
| Billing consistency ledger | `billing_usage_entries.delta_usd` |
| Balance order denomination | 普通余额订单的 `amount/refund_amount` 除以 `10`，特殊账户余额订单除以 `5`；`pay_amount` 不变 |
| Redeem and promotion | monetary `redeem_codes.value`, `promo_codes.bonus_amount`, `promo_code_usages.bonus_amount` |
| Affiliate accounting | `user_affiliates` quota/history fields and monetary/snapshot fields in `user_affiliate_ledger` |
| Batch billing | customer-denominated estimated, held, actual and billed fields, only if preflight finds historical rows |

### Keep Unchanged

| Domain | Reason |
| --- | --- |
| `payment_orders.pay_amount` | actual gateway payment |
| `RECHARGE_FEE_RATE` | payment fee percentage |
| payment min/max/daily limits | gateway fiat limits |
| subscription plan price/original price | actual product selling price |
| model catalog and raw model prices | official upstream cost basis |
| `usage_logs.total_cost` and component raw costs | upstream/raw cost basis |
| `usage_logs.account_stats_cost` and account multiplier | account-side cost accounting |
| account quota limits/usage | upstream account capacity, not user balance |
| `groups.peak_rate_multiplier` | relative factor applied after the already-scaled base multiplier |
| custom image/video base prices | raw unit prices; only the independent customer multiplier is scaled |
| affiliate rebate percentage | percentage stays the same; its monetary base is scaled |
| tokens, requests, concurrency and RPM | non-monetary quantities |

## Migration Plan

### 1. Freeze and preflight

1. Select a low-traffic window and pre-warm a parallel application instance.
2. Disable new recharge orders.
3. Confirm there are no payment orders in `PENDING`, `PAID`, `RECHARGING`, refund-pending or other in-flight states.
4. Confirm there are no running batch jobs and no active billing workers holding unsettled amounts.
5. Record row counts and sums for every field to be migrated.
6. Back up PostgreSQL, `.env`, Compose, Nginx and the current binary/resources.

### 2. Online history conversion and write drain

1. Keep the active application and Nginx online while the transaction converts historical usage rows up to a captured ID.
2. Acquire a PostgreSQL advisory lock dedicated to this migration.
3. At the end of the transaction, briefly lock `users` to drain billing transactions, capture the new usage-log high-water mark and convert the catch-up range.

### 3. Atomic conversion

Execute one reviewed SQL transaction that:

1. Converts every stored old-denomination field with decimal division by `10`, except the explicitly identified special recharge assets which divide by `5`.
2. Sets `BALANCE_RECHARGE_MULTIPLIER=1`.
3. Converts group/user-specific customer billing multipliers by `10`.
4. Leaves raw costs, pay amounts and relative factors unchanged.
5. Writes a migration marker containing scale factor, timestamp and preflight checksum.

Do not perform the migration as a sequence of admin API calls because concurrent cache state and partial failures would make it non-atomic.

### 4. Cache and service recovery

1. Invalidate only application billing/auth/quota caches; do not use an unconditional Redis `FLUSHALL`.
2. Restart the already healthy parallel instance against the migrated database so it has no old in-process rate cache.
3. Switch Nginx only after the parallel instance is healthy; then drain the old instance and reconcile any old-denomination in-flight usage rows.
4. Keep recharge disabled until all reconciliation checks pass.

### 5. Reconciliation

For each monetary aggregate, verify:

```text
abs(new_sum * 10 - old_sum) <= accepted_decimal_rounding_error
```

Verify representative users, including positive balance, negative balance, API-key quota and affiliate quota cases. Run a low-cost request in every active standard group and confirm:

```text
new_actual_cost * 10 == comparable_old_actual_cost
new_remaining_balance / new_actual_cost == old_remaining_balance / old_actual_cost
```

Then create a small recharge order and confirm payment `P` credits exactly `P` balance units before re-enabling recharge globally.

### 6. Customer-facing rollout

Publish the following explicit message before users next see their balances:

> 系统余额记账单位已由旧的放大单位统一调整为 1:1。普通历史余额及每次使用费用均同比缩小为原来的 1/10；历史特殊倍率充值已按其实际充值倍率折算。此次调整只改变显示和记账单位，不改变正常账户的可用请求量、模型实际价格或充值本金价值。此后充值本金 1 元将到账 1 个余额单位。

Keep the before/after reconciliation export available for support inquiries.

## Rollback

- Do not roll back by multiplying live values by `10`; requests written after cutover would be corrupted.
- Any failure before commit rolls the transaction back and leaves the old service/database denomination active.
- After commit, do not restore a stale backup over new writes. Keep the new denomination and forward-reconcile old-instance in-flight rows from the recorded usage-log high-water mark.
- A full backup restore is reserved for an operator-declared disaster recovery event with traffic explicitly frozen and acknowledged data-loss boundaries.

## Risks / Trade-offs

- Partial migration creates direct monetary loss or windfall. Mitigation: one DB transaction, a short end-of-transaction billing-write drain lock, and automatic rollback on any guard failure.
- Stale Redis values can apply old deductions to new balances. Mitigation: targeted billing/auth/quota cache invalidation before traffic resumes.
- Historical order ratios vary. Mitigation: restrict the exception to the two operator-confirmed accounts, derive the balance correction from completed recharge events, and record exact before/after values in an idempotent marker.
- Users may interpret the smaller balance number as lost funds. Mitigation: publish the exact before/after formula and retain a reconciliation export.
- Historical refund behavior is not part of acceptance per operator decision; no in-flight refund may exist at cutover.

## Approval Gate

The operator explicitly approved execution. Production completed the transactional cutover on 2026-07-14, followed by blue-green traffic switching, tail reconciliation and announcement publication.
