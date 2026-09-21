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
| `[CodexTurnStateSeed] invalidate … reason=server_overloaded path=… burst_attempt=N session_epoch=M` | `session_epoch` | 首次过载即清票；`burst_attempt=1` 为爆发窗口首个；`session_epoch` 随该(账号,会话)每次过载递增 |

### 跨请求轮换的观察要点

`session_epoch` 是本次新增的跨请求维度（`thread-id` 派生换到 `v2` 种子）。灰度期应确认：

1. 过载后**同一客户端会话的下一条请求**拿到不同 `thread-id`（日志侧只能看到 `session_epoch>0`；如需直接比对，可临时开启请求头审计）。
2. 窗口（30 分钟）内无新过载后，该会话回到 `v1` 确定性派生。
   若日志里出现 `thread_id_rotated=true` 但**该账号是 `device` 模式**，说明前置条件没满足——
   此时不会有任何轮换，需先切模式。
3. **prompt cache 命中率**在窗口内下降属预期；需确认下降幅度与「发生过过载的会话占比」量级一致，否则说明轮换扩散到了未过载的会话（应用 `TestOverloadEpoch_UnaffectedSessionKeepsLegacyDerivation` 的逻辑排查）。

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

### 前置条件（必须先确认，否则实验无效）

**轮换只在 `session` / `full` 指纹模式下有对象。** `device` 模式在
`applyCodexFingerprintHeaders` 里提前 return，只写 `x-codex-installation-id`，**完全不碰
`thread-id`**，因此 `resolveConvergedThreadID` 不会被调用。

当前生产 5 个活跃 OpenAI OAuth 账号全部是 `device` 模式 → **上线前必须先把它们切到
`session`**（`extra.codex_fingerprint_mode=session`）。`session` 是
`add-codex-fingerprint-convergence` 里「新建 OAuth 账号的默认值」，属于受支持的目标配置；
但切模式本身会改变上游可见指纹（1 设备 + 1 会话 + N 线程），所以两组必须都切，
只用下面的开关区分，否则两个变量混在一起无法归因。

### 分组（账号级覆盖）

| 组 | 账号 `extra` | 行为 |
|---|---|---|
| A（对照） | `codex_overload_identity_rotation=off` | 旧行为：过载只做同账号原样重试，票需连续 3 次 5xx 才清 |
| B（实验） | `codex_overload_identity_rotation=on`（或不设置，跟随全局） | 首次过载即清票 + 同账号重试/跨请求轮换 `thread-id` + 爆发窗口 +2 重试预算 |

全局配置 `gateway.disable_codex_overload_identity_rotation=true` 仍可一键整体回滚；
账号级覆盖优先于全局（便于单账号紧急退出）。

### 验收指标

以「上游过载后的同账号重试」为分母统计：

1. **主指标**：同账号重试后的请求成功率
   - 基线预期 ≈ 20%（实测同会话重试 19.6%，n=46）
   - 目标 ≥ 60%（实测新会话重试 65.7%，n=35）
2. **辅指标**：用户可见 503 / `Selected model is at capacity` 次数
3. **代价指标**（应无显著恶化）：
   - 上游请求总量（重试预算放宽最多 +2 次/请求，且仅在爆发窗口内）
   - prompt cache 命中率：**跨请求 epoch 会让发生过过载的会话在窗口内丢缓存**，下降属预期；需与「发生过过载的会话占比」量级一致
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
