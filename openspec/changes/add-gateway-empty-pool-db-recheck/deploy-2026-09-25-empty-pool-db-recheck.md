# 生产发布记录：add-gateway-empty-pool-db-recheck（网关空池强制 DB 复核）

- 日期：2026-09-25（记录用 +08:00；生产机时区 JST +09:00，同一时刻 = +1 小时）
- 代码：`c1d5ac415 feat: 网关空池强制 DB 复核 + 移除 Claude OAuth 登录能力`
- change：`openspec/changes/add-gateway-empty-pool-db-recheck/`
- 生产机：`sub2api-prod`（74.82.195.84），域名 `code.sicts.shop`
- 与 `remove-claude-oauth-login` **同批、同一后端二进制**（`9535ad651`）；本 change 无前端改动、无 schema 变更、无环境变量变更。

## 0. 变更语义（一句话）

网关选号路径（Anthropic `/v1/messages`、Gemini、Antigravity）此前只读调度快照，第一跳看到空池即失败；
本次补齐：**空池且无排除账号时**强制回 DB 重建一次 bucket 快照并重试选号一次，仍失败则保留原错误分类
（见 `backend/internal/service/gateway_empty_pool_recheck.go`）。

## 1. 发布顺序与产物

后端单侧发布（前端无改动；同一窗口的前端 release 属于 `remove-claude-oauth-login`，
见 `openspec/changes/remove-claude-oauth-login/deploy-2026-09-25-claude-oauth-retirement.md`）。
后端沿用既有约定：**同容器热替换 bind-mount 二进制**（`docker stop` → 覆盖文件 → `docker start`）。

### 1.1 后端

| 项 | 值 |
| --- | --- |
| 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags embed ./cmd/server`（go1.26.4） |
| 二进制 | `sha256=6e017abd1dbaafce1767d53dd8e97ec4e64055a55f24d9ba4eae21d9a39ff163`，123,682,978 B |
| 版本指纹 | `go version -m` → `-tags=embed`、`-trimpath=true`、`CGO_ENABLED=0`、`GOOS=linux`、`GOARCH=amd64`、`vcs.revision=9535ad65175b85aeb918f3ac87eb1a8eb4d4188f`、`vcs.time=2026-09-24T19:35:45Z`、`vcs.modified=true` |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`127.0.0.1:18373` → nginx upstream `sub2api_api`） |
| 挂载点 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api → /app/sub2api` |
| 热替换 | `docker stop` → `install -m 0755` 覆盖 → `docker start`；`StartedAt=2026-09-24T19:37:21Z`（= 03:37 +08:00 / 04:37 JST），`FinishedAt=19:37:20Z`，`RestartCount=0`，`healthy` |
| 本 change 首次在线 | `378f920ca` 已包含本 change 代码（`git merge-base --is-ancestor c1d5ac415 378f920ca` = true），该 revision 的上线不晚于当前容器重启（`sub2api.bak-20260925-claudeoauthretire2` 即其备份）；当前二进制 `9535ad651` 为其超集。本节核验事件均取自 `9535ad651` 当前态 |
| 生效时刻 | 末尾日志尾部告警 `security.url_allowlist.enabled=false` 出现于 `2026-09-25T03:37:21.723+08:00`，与 `StartedAt` 对齐 |

### 1.2 回滚点

| 目标 | 文件 | revision | sha256 |
| --- | --- | --- | --- |
| 只撤销本 change（同时撤销 Claude OAuth 退役） | `sub2api.bak-20260925-claudeoauthretire` | `19d1d927c` | `8797e9694c5c9aa3…` |
| 撤销前端收口批次（仍含本 change） | `sub2api.bak-20260925-claudeoauthretire2` | `378f920ca` | `91b7047740e33ac4…` |

> 本 change 与 `remove-claude-oauth-login` 共用同一二进制，因此**不存在「只回滚本 change 而保留退役」的点**；
> 若要单独撤销本 change 需重新构建。回滚操作：`docker stop` → `install -m 0755 <bak> …/sub2api` → `docker start`。

## 2. 发布后核验（只读）

窗口：`docker logs --since 2026-09-24T19:37:21Z sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，
覆盖 `2026-09-25T03:37:22+08:00` → `03:56:21+08:00`（约 19 分钟，2,138 行）。

### 2.1 二进制标记

线上二进制 `strings -a` 命中：`gateway_scheduler_empty_pool_db_recheck` = 3（含 `_unrecovered` / `_failed` 事件名）、
`gateway_scheduler_empty_pool_db_recheck_unrecovered` = 1、`gateway_scheduler_empty_pool_db_recheck_failed` = 1
→ 新代码确已在线。

### 2.2 空池复核事件（3.2 前半）

| 事件 | 次数 |
| --- | --- |
| `gateway_scheduler_empty_pool_db_recheck` | **1**（真正触发复核） |
| `gateway_scheduler_empty_pool_db_recheck_unrecovered` | 1（复核后重试仍失败） |
| `gateway_scheduler_empty_pool_db_recheck_failed` | 0 |
| `openai_scheduler_empty_pool_db_recheck` | 0（OpenAI 既有复核路径未受影响/未触发） |
| `gateway.forward_failed` | 0 |

原始日志行：

```
{"level":"WARN","time":"2026-09-25T03:47:29.085+0800","msg":"gateway_scheduler_empty_pool_db_recheck",
 "platform":"anthropic","group_id":34,"model":"deepseek-v4-pro",
 "snapshot_pool_size":2,"snapshot_candidate_count":0,"db_candidate_count":2}
{"level":"WARN","time":"2026-09-25T03:47:29.087+0800","msg":"gateway_scheduler_empty_pool_db_recheck_unrecovered",
 "platform":"anthropic","group_id":34,"model":"deepseek-v4-pro",
 "db_pool_size":2,"db_candidate_count":0,"error":"no available accounts"}
```

即：快照池有 2 个账号但候选为 0 → 触发一次 DB 复核（DB 侧返回 2 个候选）→ 重试仍为 0 候选 →
按既有错误分类（`no available accounts`）失败。日志仅含 `group_id`/`platform`/`model`/计数，无上游报文与凭据。

### 2.3 「未恢复」归因：真实不可用，非回归

`group 34` 当前可用账号（`accounts ⨝ account_groups`，`deleted_at IS NULL`，`schedulable = t`）：

| id | name | platform | type | `credentials.model_mapping`（键） |
| --- | --- | --- | --- | --- |
| 81 | 国模 | anthropic | apikey | `k3`、`glm-5.1`、`glm-5.2`、`glm-5.3`、`kimi-k3`、`mimo-v2.5`、`qwen3.7-max`、`qwen3.8-max`、`glm-5.3-flash`、`mimo-v2.5-pro`、`qwen3.8-flash` |
| 139 | OpenCode Go | anthropic | apikey | `deepseek-v4.1-flash` |
| 140 | mirasim-relay | anthropic | apikey | `kimi-k3`、`glm-5.3-flash`、`deepseek-flash`、`deepseek-v4-flash`、`deepseek-v4.1-flash`、`deepseek-v4-flash-vision-exp` |

**没有任何账号映射 `deepseek-v4-pro`** → 复核后 DB 候选仍为 0 属预期，失败判定正确；
`gateway_scheduler_no_candidates_reject_trace` = 2（既有 no-candidate 追踪路径，非新增分支）。

### 2.4 正常请求无新增延迟（3.2 后半）

| 视角 | 证据 |
| --- | --- |
| 门禁（代码级） | 复核仅在「本次选号已失败（`errors.Is(err, ErrNoAvailableAccounts)`）**且**池/候选为空 **且** `len(excludedIDs)==0`」时触发；成功路径 `err == nil` 立即返回（`gateway_empty_pool_recheck.go:88-97`）→ 正常请求不产生额外 DB 读、不新增锁、不写日志 |
| 观测比例 | 窗口内网关选号 138 次（`account_simple_selection`）、`sticky.account_selected` 135 次，仅 **1** 次复核；`gateway.forward_failed` = 0 |
| 单次开销 | 唯一一次复核 + 重试的额外耗时 = 日志时间戳差 **2ms**（`03:47:29.085` → `03:47:29.087`） |
| 限流沿用 | 复核走既有 `SchedulerSnapshotService.RefreshSchedulableAccounts`（per-bucket 节流 `schedulerForcedRefreshGap` + DB fallback 限流器），窗口内重复调用返回 `ErrSchedulerFallbackLimited` 时静默放弃；未新增开关/节流参数 |
| 限制 | 线上无 Prometheus 端点（`GET /metrics` 返回 SPA HTML，714 B），无法给出改前/改后 p95 对比；故以「非失败路径不可达该分支」+ 触发计数 + 单次耗时作证 |

### 2.5 单测（覆盖生产未出现的「复核后恢复」分支）

`cd backend && go test ./internal/service/ -run 'Recheck' -count=1` → **ok**，14 个用例全部 PASS，含：

- `TestSelectGatewayAccountWithEmptyPoolRecheckRefreshesAndRetries`（复核后恢复并重试成功 —— 生产窗口未出现该形态）
- `TestSelectGatewayAccountWithEmptyPoolRecheckLogsUnrecovered`
- `TestSelectGatewayAccountWithEmptyPoolRecheckSkipsWhenAccountsExcluded`（有排除账号不复核）
- `TestSelectGatewayAccountWithEmptyPoolRecheckSkipsBusyPool`（忙池不复核）
- `TestSelectGatewayAccountWithEmptyPoolRecheckLeavesOtherErrorsUntouched`（非空池错误不处理）
- `TestRecheckEmptyGatewayPoolThrottlesRepeatedRefreshes`（节流不复核）
- `TestRecheckEmptyGatewayPoolWithoutSnapshotService` / `TestSelectGatewayAccountWithEmptyPoolRecheckWithoutSnapshotService`
- 既有 OpenAI 复核用例 4 个（`..._SessionStickyDBRuntimeRecheckSkipsStaleCachedAccount` 等）保持通过 → 未破坏原路径

## 3. 回滚

1. 后端：`docker stop sub2api-api-candidate-0.1.391-noinject-a33b47ef9` →
   `install -m 0755 /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-claudeoauthretire …/sub2api` →
   `docker start`；核验 `go version -m` → `vcs.revision=19d1d927c`。
2. 该回滚会一并撤销 `remove-claude-oauth-login` 的退役与 `378f920ca`/`b8a8b66ad`/`9535ad651` 三个提交的改动；
   如需保留退役只回滚本 change，必须重新构建。
3. 无 schema/Redis 迁移需回退：本 change 不写库、不改 Redis 键结构（仅走既有快照刷新）。

## 4. 遗留与观察项

- 「复核后恢复」在生产尚未被自然触发（19 分钟 138 次选号仅 1 次空池且属真实不可用），仅由单测覆盖；
  后续若出现 `..._db_recheck` 且**无**对应 `..._unrecovered`，即为该分支的生产正向实例。
- 复核成本上限 = 一次 bucket 快照刷新 + 一次完整选号重试；`..._failed` 与 `..._unrecovered` 均按
  WARN 记录以便长期观察触发频率。
- 本次未推送远端（本地提交沿用既有节奏）。
