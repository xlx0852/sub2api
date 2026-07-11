## Context

旧单位下，支付金额被放大为余额，用户实际扣费也按同样的放大单位记录。本次变更是记账面值归一，不是降价、涨价或改变上游成本。

## Goals / Non-Goals

### Goals

- 新用户余额与充值金额按 `1:1` 表达。
- 迁移前后每个用户的可调用量不变。
- 每个模型、分组和媒体类能力的相对售价不变。
- 平台同等实付金额可承载的上游成本和利润不变。
- 用户在余额、API Key 限额、用量记录和邀请返利页面看到一致的新单位。

### Non-Goals

- 不改变模型官方定价或 catalog/pricing JSON。
- 不改变支付通道币种、支付手续费率、最低/最高实付限制。
- 不改变账号侧成本统计倍率和上游账号配额。
- 不在本次变更中重新设计历史退款业务。

## Core Invariant

设旧单位放大因子 `S = 10`：

```text
new_balance = old_balance / S
new_customer_charge = old_customer_charge / S
new_recharge_multiplier = old_effective_recharge_multiplier / S = 1
```

用户可调用量保持不变：

```text
old_balance / old_customer_charge
  = (old_balance / S) / (old_customer_charge / S)
  = new_balance / new_customer_charge
```

这一等式是切换的唯一业务验收标准。

## Data Classification

### Divide by 10

| Domain | Fields |
| --- | --- |
| User funds | `users.balance`, `users.frozen_balance`, `users.total_recharged`; fixed `balance_notify_threshold` |
| Default grants | `default_balance` and every `auth_source_default_*_balance` setting |
| API key quota | `api_keys.quota`, `api_keys.quota_used` |
| Platform quota | all limit and usage fields in `user_platform_quotas` |
| Customer billing rate | `groups.rate_multiplier`; `user_group_rate_multipliers.rate_multiplier` |
| Independent media rate | `groups.image_rate_multiplier` only when `image_rate_independent=true`; `groups.video_rate_multiplier` only when `video_rate_independent=true` |
| Subscription quota | non-zero group daily/weekly/monthly limits and corresponding `user_subscriptions.*_usage_usd` |
| Customer usage history | `usage_logs.actual_cost`, `usage_logs.rate_multiplier`; dashboard hourly/daily `actual_cost` |
| Balance order denomination | `payment_orders.amount` and balance-order `refund_amount`; `pay_amount` remains unchanged |
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

1. Select a low-traffic window and publish a short maintenance notice.
2. Disable new recharge orders.
3. Confirm there are no payment orders in `PENDING`, `PAID`, `RECHARGING`, refund-pending or other in-flight states.
4. Confirm there are no running batch jobs and no active billing workers holding unsettled amounts.
5. Record row counts and sums for every field to be migrated.
6. Back up PostgreSQL, `.env`, Compose, Nginx and the current binary/resources.

### 2. Stop writes

1. Route Nginx to a maintenance response or stop the application after draining requests.
2. Verify no new `usage_logs`, payment orders or affiliate ledger rows are being written.
3. Acquire a PostgreSQL advisory lock dedicated to this migration.

### 3. Atomic conversion

Execute one reviewed SQL transaction that:

1. Converts every stored old-denomination field with decimal division by `10`.
2. Sets `BALANCE_RECHARGE_MULTIPLIER=1`.
3. Converts group/user-specific customer billing multipliers by `10`.
4. Leaves raw costs, pay amounts and relative factors unchanged.
5. Writes a migration marker containing scale factor, timestamp and preflight checksum.

Do not perform the migration as a sequence of admin API calls because concurrent cache state and partial failures would make it non-atomic.

### 4. Cache and service recovery

1. Invalidate only application billing/auth/quota caches; do not use an unconditional Redis `FLUSHALL`.
2. Start a new application instance against the migrated database.
3. Keep recharge disabled until all reconciliation checks pass.

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

> 系统已将余额记账单位从 1:10 统一为 1:1。您的余额数字和每次使用费用均同比缩小为原来的 1/10，可用请求量和实际价格不变。例如，原余额 1000、单次扣费 10，调整后为余额 100、单次扣费 1。

Keep the before/after reconciliation export available for support inquiries.

## Rollback

- Do not roll back by multiplying live values by `10`; requests written after cutover would be corrupted.
- If validation fails before reopening traffic, keep writes stopped and restore the complete pre-migration PostgreSQL backup plus the previous application/config bundle.
- Restore the old Nginx upstream only after the old database and old `BALANCE_RECHARGE_MULTIPLIER` are both active.

## Risks / Trade-offs

- Partial migration creates direct monetary loss or windfall. Mitigation: one DB transaction with writes stopped.
- Stale Redis values can apply old deductions to new balances. Mitigation: targeted billing/auth/quota cache invalidation before traffic resumes.
- Old `1:5` order records do not represent the manual 50% compensation alone. Mitigation: use the confirmed effective `1:10` denomination globally, including admin balance adjustments, instead of deriving user balances from payment orders.
- Users may interpret the smaller balance number as lost funds. Mitigation: publish the exact before/after formula and retain a reconciliation export.
- Historical refund behavior is not part of acceptance per operator decision; no in-flight refund may exist at cutover.

## Approval Gate

This document records the plan only. No setting, database row, multiplier or production service shall be changed until the operator explicitly approves execution during a low-traffic window.
