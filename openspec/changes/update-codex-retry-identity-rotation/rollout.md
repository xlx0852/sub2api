## 灰度观察

本变更只改「上游明确报告容量过载之后」的恢复行为，不改变选择逻辑、并发限制与计费语义。默认开启，可用一个开关整体回滚。

### 关键日志与字段

| 位置 | 字段 | 期望 |
|---|---|---|
| `openai.pool_mode_same_account_retry` | `thread_id_rotated=true` | 同账号重试已轮换 `thread-id` |
| 同上 | `fingerprint_attempt=N` | 轮次随每次同账号重试递增（1,2,3…） |
| 同上 | `overload_path=http_503` / `stream_failed` | 过载通道；两者都表示同一错误码 |
| `openai_messages.pool_mode_same_account_retry` | 同上 | Anthropic Messages 入口 |
| `openai_chat_completions.pool_mode_same_account_retry` | 同上 | Chat Completions 入口 |
| `openai_alpha_search.pool_mode_same_account_retry` | 同上 | Alpha Search 入口 |
| `reconnect_retry`（WS） | `reason=server_overloaded`、`thread_id_rotated` | WS 重连重试的轮换 |
| `[CodexTurnStateSeed] invalidate … reason=server_overloaded path=… burst_attempt=N` | `burst_attempt` | 首次过载即清票；`burst_attempt=1` 为爆发窗口首个 |

### 必须为 0 的异常

- `thread_id_rotated=false` 却出现 `fingerprint_attempt>0`：轮换未生效（指纹模式可能是 `off`/`device`/`full`，见下文「指纹模式前提」）。
- 同一请求日志里出现两次 `fingerprint_attempt=1`：轮次未累计，说明有调用点没走 `BumpCodexFingerprintAttempt`。
- `burst_attempt` 无界增长：爆发窗口未按 30 分钟重置。

### 指纹模式前提

轮换只作用于 `session` 模式（`extra.codex_fingerprint_mode=session`）下派生的 `thread-id`：

- `off`：完全不改写身份，轮换无对象（`thread_id_rotated=false` 属正常）。
- `device`：不写 `thread-id`，无轮换。
- `session`：**本变更的目标场景**。
- `full`：`thread-id` 恒等于账号级 `session-id`，按设计**不轮换**（该模式的语义就是全收敛）。

灰度前先确认目标分组账号的 `codex_fingerprint_mode`。

## A/B 方案

同一分组内按账号二选一，运行 1-2 天，尽量覆盖不同时段（过载有日级潮汐：多数日子 0%，个别日高达 70%）。

| 组 | 账号 `extra` | 行为 |
|---|---|---|
| A（对照） | `disable_codex_overload_identity_rotation=true` | 旧行为：过载只做同账号原样重试，票需连续 3 次 5xx 才清 |
| B（实验） | 不设置（默认） | 首次过载即清票 + 同账号重试轮换 `thread-id` + 爆发窗口 +2 重试预算 |

也可用配置整体切换（`gateway.disable_codex_overload_identity_rotation`）做整组对照，但账号级对照能天然控制时段差异。

### 验收指标

以「上游过载后的同账号重试」为分母统计：

1. **主指标**：同账号重试后的请求成功率
   - 基线预期 ≈ 20%（实测同会话重试 19.6%，n=46）
   - 目标 ≥ 60%（实测新会话重试 65.7%，n=35）
2. **辅指标**：用户可见 503 / `Selected model is at capacity` 次数
3. **代价指标**（应无显著恶化）：
   - 上游请求总量（重试预算放宽最多 +2 次/请求，且仅在爆发窗口内）
   - prompt cache 命中率（轮换会丢缓存，但仅发生在失败之后）
   - 单请求尾延迟

### 统计口径

- 分子/分母都取自带 `thread_id_rotated` 字段的结构化日志，避免混入非过载重试。
- 过载有日级潮汐，**不要跨日直接比总量**；按账号对照或按同一 30 分钟窗口配对比较。
- 样本量提醒：本地全期过载率 0.94%（190/20159），A/B 需要足够的过载样本才有结论；若短期样本不足，优先看 `thread_id_rotated` 是否按预期出现（功能正确性），再看成功率（效果）。

## 回滚

1. **单账号**：给账号 `extra` 设 `disable_codex_overload_identity_rotation=true`，立即退回旧行为，无需重启。
2. **整组/全局**：配置 `gateway.disable_codex_overload_identity_rotation=true` 后重载配置。
3. 回滚不涉及数据库迁移与持久化数据修正：爆发窗口追踪是纯内存状态（进程内 map，30 分钟自然过期，且有 4096 条上限），无 schema 变更。

## 停止条件

出现任一情况立即回滚：

- 重复计费或计费金额异常（轮换不应影响计费；计费仍以实际成功流结算）。
- prompt cache 命中率显著下降 **且** 跌幅无法用「失败重试次数」解释（说明轮换扩散到了首次尝试）。
- 转号后账号健康指标恶化（过载是请求级信号，本变更不触碰账号健康；若健康被牵连说明分类有误）。
- 上游请求总量增幅明显超过「爆发窗口内 +2 次重试」的上界。

## 已知边界

- 爆发窗口预算只放宽**同账号重试上限**（默认 +2，且要求窗口内已出现 ≥2 次过载），不放宽跨账号切换上限。
- 流内过载（`overload_path=stream_failed`）仍受既有「已输出可见内容不重放」边界约束；长尾场景（实测最长 86 分钟后才 `response.failed`）不会重放，只清票与轮换，供**下一次**请求使用。
- 通用 failover 路径（`FailoverState.HandleFailoverError`，Gemini 等平台）本次未接入轮换：该路径换账号时 `account.ID` 变化本身就会改变 `thread-id`。若后续观测到该路径存在同账号重试行为，再单独评估。
