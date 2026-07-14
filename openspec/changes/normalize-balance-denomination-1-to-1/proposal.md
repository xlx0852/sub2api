# Change: 余额记账单位从 1:10 归一为 1:1

## Why

当前业务实际按“支付 1 元获得 10 个余额单位”运行，用户扣费也使用同一放大单位。这会让余额、费率和历史数据的理解成本增加。目标是将记账单位统一为 1:1，同时保持用户可调用量、实际支付和平台利润不变。

## What Changes

- **BREAKING** 将普通用户余额、限额、用户实扣费和相关历史金额统一除以 `10`。
- 对两个历史 `1:5` 充值账户 `s928215036@gmail.com`、`shizeying@joyme.sg`，充值入账与订单除以 `5`；基础赠送、使用费率和使用记录仍按全站口径除以 `10`。
- **BREAKING** 将标准分组和用户专属扣费倍率统一除以 `10`。
- 将余额充值倍率设为 `1`，之后支付 `P` 元到账 `P` 个新余额单位。
- 保留官方模型价格、原始上游成本、支付通道实付金额、手续费率和订阅售价不变。
- 通过在线历史更新、末段短数据库锁、单事务数据迁移、蓝绿切流、缓存失效和尾账对账完成切换，不停止对外服务。
- 对用户发布“余额和单价同比缩小 10 倍，可用量不变”的公告。

## Impact

- Affected specs: `balance-denomination`
- Affected code: payment config, balance billing, group multipliers, usage logs, quota and affiliate accounting
- Affected data: users, API keys, groups, platform quotas, subscriptions, usage logs, payment orders, redeem codes, affiliate quota/ledger, aggregated usage statistics
- Operational impact: requires a full PostgreSQL backup, a pre-warmed parallel instance, a short billing-write drain lock and targeted cache invalidation
- Explicit non-goal: this change does not redesign or separately validate historical refund policy

## Current Production Baseline (2026-07-11)

- Business baseline confirmed by operator: effective historical denomination is `1:10` for normal accounts; the two named accounts use an explicit `1:5` recharge-credit exception.
- Current visible setting is not authoritative for migration math; all old monetary units are treated as `10 old units = 1 new unit`.
- Production `BALANCE_RECHARGE_MULTIPLIER` was re-verified as `10.00` on 2026-07-14 and shall become `1.00` at cutover.
- Current standard group multiplier conversion:
  - `codex`: `1.0 -> 0.1`
  - `Claude`: `10.0 -> 1.0`
  - `codex特惠`: `0.6 -> 0.06`
  - `cc 特惠`: `2.0 -> 0.2`
  - `Grok`: `10.0 -> 1.0`
- There are currently no user-specific group multiplier rows, promo bonuses or batch image jobs, but the migration must re-check these conditions immediately before cutover.
