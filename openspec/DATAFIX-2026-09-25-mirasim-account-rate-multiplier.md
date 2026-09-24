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

## 追加修复：经营汇总用 UTC 日期取区间，漏掉当天用量（前端，2026-09-25 06:13）

**现象**：倍率快照修正后，用量详情弹窗「经营汇总」仍显示 收入 $0.0000 / 成本 $0.0000 / 利润 +$0.0000 / 请求数 0，而同页「30 天总请求」是 397、「今日概览」用户扣费 $1.52。

**原因**：`AccountStatsModal` 用 `start/end.toISOString().slice(0, 10)` 生成区间字符串（**UTC 日期**），而接口按请求携带的 `timezone`（浏览器时区）解释 `end_date`。东八区 0:00–8:00 期间本地日期比 UTC 晚一天，`end_date` 落在昨天，当天（09-25 01:40–05:39 +08，共 397 行）被整段排除。

实测（`timezone=Asia/Shanghai`，`start_date=2026-08-26`）：

| end_date | requests | revenue | cost | profit |
| --- | --- | --- | --- | --- |
| 2026-09-24 | 0 | 0 | 0 | 0 |
| 2026-09-25 | 397 | 1.5238 | 0 | 1.5238 |

**修复**（提交 `01a904fb3`）：改用 `utils/format.formatDateLocalInput`（本地日期）——

- `AccountStatsModal.vue`：经营汇总区间
- `ProfitView.vue`：默认区间与 today/7d/30d/month 预设（同类问题：0:00–8:00 期间区间整体少一天）
- `AccountCostConfigPanel.vue`：订阅周期起始日推算（`setUTCDate` → `setDate`）
- 新增回归测试：UTC 与本地不在同一天时必须使用本地日期；已验证把实现改回 `toISOString()` 时该用例失败

**发布**：前端 release `/opt/sub2api/frontend/releases/0.1.391-01a904fb3`（聚合 `b74333bdfaf1aff80d5b88eab03ec2fb762926ab6f2c09fdee7ee8b8ff7fcaa6`，171 文件，AppleDouble 0；`index.html` = `2604ac8dc987a072ee22637513dcc1aaa4816d2666b66cfafca3bfde13ca8f25`，入口 `/assets/index-4EXEkrs9.js`）。`current` → `0.1.391-01a904fb3`，`rollback` → `0.1.391-756ee2ecc`。后端二进制未变更。

**核验**：`GET /admin/profit/summary?start_date=2026-08-26&end_date=2026-09-25&timezone=Asia/Shanghai&account_id=140` → requests 397 / revenue 1.5238 / **cost 0** / **profit +1.5238** / margin 100%。同一提交后，`vue-tsc --noEmit`、`pnpm build` 通过，vitest 失败集与基线一致（`UserPlatformQuotaModal` ×2、`AccountsView.sparkShadow` ×1）。

## 后端补发：把 06:12 的 use? 提交一并上线（2026-09-25 06:15）

**背景**：提交 `3c0292efe feat(usage): add request timing detail view` 完成于 06:12 +08，晚于 05:42 部署的后端二进制。前端（`0.1.391-01a904fb3`）已包含「请求耗时详情」弹窗，而运行中的后端仍在 `756ee2ecc`，`usage_logs` DTO 缺 `first_output_stages`，弹窗只能显示占位。

**处置**：按当前 HEAD `9579dc923` 重建后端并热替换。

| 项 | 值 |
| --- | --- |
| 构建 | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=true -tags embed ./cmd/server`（前端 dist 为 06:13 构建，与前端 release 一致） |
| 制品 | 123,696,936 B，`sha256=7b646b3cf272c4e83e4987d148902dd558a1c3d3ca6cc5e327579727778ec9a1`，`vcs.revision=9579dc9234d397e563cfe577b62b9aa77bc92ba0` |
| 备份 | `sub2api.bak-20260925-timing`（替换前 `sha256=5339fd48…`） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，`Up (healthy)`、`RestartCount=0` |

**核验**：

- `GET /api/v1/admin/usage?account_id=126&page_size=3` → 每行含 `first_output_stages`（例：`{"gateway_pre_upstream_ms":31,"upstream_connect_ms":0,"upstream_first_output_ms":2078,...}`）；无数据的行因 `omitempty` 不返回该字段。
- 利润分析配额窗口接口与 ops token 统计接口仍为 **404**（保持本次移除）；`/health` = 200；替换后 5 分钟 5xx = 0。
- `profit/summary?...&account_id=140&timezone=Asia/Shanghai` → requests 397 / revenue 1.5238 / **cost 0** / **profit +1.5238** / margin 100%。
