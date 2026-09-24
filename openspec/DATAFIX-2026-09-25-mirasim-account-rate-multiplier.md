# 生产数据修正：账号 140（mirasim-relay）账号倍率快照（2026-09-25）

## 现象

账号 140「mirasim-relay」在利润「经营汇总」里显示 收入 $1.5238 / 成本 $15.2361 / 利润 −$13.7123 / 利润率 −899.9%。

## 归因

- 成本口径为 `SUM(total_cost × account_rate_multiplier 快照)`，收入为 `SUM(actual_cost)`；快照在写用量行时取自账号当时的「综合成本倍率」（`account.BillingRateMultiplier()`）。
- 140 的 397 行用量（全部为 2026-09-25 01:40–05:39 +08）快照均为 **1.0**，即按模型标价计入成本；其中 kimi-k3 缓存读取占 $11.41（38.0M tokens × $0.30/1M），输入 $1.65、输出 $0.80，deepseek-v4.1-flash $1.36。
- 分组「国模集合」售价倍率 0.1，故收入仅为标价的 10%。
- 账号的「综合成本倍率」在 **06:07:06 +08** 被改为 **0**，但既有的历史行保留原快照，因此汇总不变。并非计算错误，而是快照口径不回溯。

## 处置

生产库 `sub2api`（容器 `sub2api-postgres`）执行：

1. 备份：`CREATE TABLE usage_logs_ratemult_backup_20260925 AS SELECT id, account_id, account_rate_multiplier, total_cost, actual_cost FROM usage_logs WHERE account_id = 140;` → **397 行**。
2. 对齐：`UPDATE usage_logs u SET account_rate_multiplier = a.rate_multiplier FROM accounts a WHERE a.id = 140 AND u.account_id = 140 AND COALESCE(u.account_rate_multiplier, -1) IS DISTINCT FROM a.rate_multiplier;` → **UPDATE 397**，快照全部变为账号当前值 `0.0000`。
3. 事务内完成（`BEGIN` / `COMMIT`），未改动 token、标价与用户扣费字段。

## 核验（只读）

| 接口 | 修正前 | 修正后 |
| --- | --- | --- |
| `GET /admin/profit/summary?...&account_id=140` | revenue 1.5238 / cost 15.2361 / profit −13.7123 / margin −899.9% | revenue 1.5238 / **cost 0** / **profit +1.5238** / **margin 100%** |
| `GET /admin/accounts/140/stats?days=30` | total_cost 15.236062 | total_cost 0，`total_standard_cost` 仍为 15.236062（标价口径保留） |

## 回滚与后续

- 回滚：`UPDATE usage_logs u SET account_rate_multiplier = b.account_rate_multiplier FROM usage_logs_ratemult_backup_20260925 b WHERE u.id = b.id;`
- 之后的新请求按账号当前倍率（0）记录，口径持续一致。
- 若后续为 140 设定更贴近 mirasim 实付价的倍率（例如 0.1），可用同一条「对齐」SQL 重跑；备份表 `usage_logs_ratemult_backup_20260925` 保留原值。
- 倍率 0 表示该账号的 token 成本不计入经营成本；账号用量页仍显示标价成本（`total_standard_cost`）供参考。
