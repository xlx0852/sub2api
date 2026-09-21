# Change: 过载重试时轮换 Codex 会话身份

## Why

生产与本地实测都显示 `server_is_overloaded`（客户端 UI 文案 `Selected model is at capacity. Please try a different model.`）是**上游容量的瞬时抽签**，与 IP、账号、票据 TTL、payload 大小无关；但它**高度黏着在会话身份上**。

对 20159 个历史 turn 的回溯分析（190 次过载事件，`scripts/codex_overload_report.py`）显示：过载后**仍在同一会话标识上重试，成功率只有 19.6%；换新会话标识重试则有 65.7%**（同 30 分钟窗口内对照，3.36×；14 个窗口中 10 个新会话更好、3 个持平）。按爆发深度分桶、按 payload 大小分桶后该优势依然存在。

当前网关恰好落在低成功率那一档：

1. `backend/internal/service/openai_codex_fingerprint.go:105` 的 `thread-id` 由 `f(account.ID, clientSessionID)` **确定性派生**，同账号重试得到完全相同的 `thread-id`。
2. `backend/internal/handler/failover_loop.go:117` 的同账号重试只 `sleep(500ms)` 后 `continue`，**不改变任何身份字段**。
3. `backend/internal/service/openai_codex_turn_state.go:473` 的 `openAICodexTurnStateUpstream5xxInvalidateThreshold = 3` 要求**连续 3 次 5xx** 才丢弃被钉住的 `x-codex-turn-state`（另有 2 分钟冷却），而实测过载是瞬时抽签，第 1 次失败后就应重抽。

## What Changes

- **同账号重试时轮换派生的 `thread-id`**（加入 attempt 序号），使重试获得新的上游路由；`session_id` 保持账号级恒定不变。
- **首次 `server_is_overloaded` 即刻丢弃该账号/池的 `x-codex-turn-state`**，不再等待连续 3 次 5xx，且不受失效冷却阻塞。
- **流内过载纳入同一恢复路径**：HTTP 200 + SSE `response.failed`（`error.code == "server_is_overloaded"`）与 HTTP 503 采用相同的重抽决策，仍遵守"已输出可见内容后不重放"边界。
- **过载爆发窗口预算**：一次过载后在有界时间窗内提高该账号组的重试/轮换预算（实测爆发簇最大 13 次 / 42 分钟）。
- 增加结构化观测字段：`thread_id_rotated`、`turn_state_discarded`、`overload_path`（`http_503` / `stream_failed`）、`burst_attempt_index`。

## Non-Goals

- **不引入"换模型降级"作为过载对策**。原始数据里 `gpt-6-astra` 过载率 13.82% 远高于其他模型，但这是时间混淆：在同 30 分钟窗口内对照，astra 仅比其他模型高 **1.39×**。真正起作用的是会话标识，不是模型。
- **不轮换 `session_id`**。`openai_codex_fingerprint.go:18` 注释说明 `session_id` 是刻意收敛为账号级常量、用于控制上游"设备数/会话数"判定的；而 `thread-id` 本就按客户端会话变化（`openai_codex_fingerprint.go:28-29`），在其上加 attempt 序号风险最低。
- **不引入基于延迟的提前预警**。实测最优阈值（滚动 30 分钟 p50 TTFT > 2.5× 基线）精确率仅 18.3%、召回 10.0%，会产生海量误报。
- 不改动并发限制、计费语义与 `codex_cli_only` 行为。

## Impact

- Affected specs: `openai-gateway`（与 pending change `update-openai-overload-failover` 同一 capability，需求名不重叠）
- Affected code:
  - `backend/internal/service/openai_codex_fingerprint.go`（派生 `thread-id` 时接受 attempt）
  - `backend/internal/service/openai_codex_turn_state.go`（即时丢弃路径）
  - `backend/internal/handler/failover_loop.go`（同账号重试传递 attempt）
  - "已输出内容不重放"的流边界判定（复用现有 `writerSizeBeforeForward` 语义）
  - 相关 Go 单测
- Compatibility: 客户端 API 形状不变；重试会增加 prompt cache miss（重试本无产出，可接受）；上游观测的 thread 数会略增（该维度本就按客户端会话变化，非账号级敏感字段）
- Rollout: 先 `test` 分组灰度，对比"重试成功率 / 用户可见 503 / 重复计费"

## Related changes

- `update-openai-overload-failover`（pending，9/11）：引入「跨账号恢复」与「流重放安全」两项需求。本变更**不修改**这两项，只在其基础上增加「重试时的会话身份轮换」与「即时丢弃被钉住的票」。两者同属 `openai-gateway` capability，需求名不重叠。
- `harden-openai-pool-outcome-sticky-stream`（pending，26/30）：定义了 `previous_response_id` 亲和与三层调度元数据。本变更的 `thread-id` 轮换与其中的会话亲和机制同向（都认为会话/线程标识决定上游路由），但作用于**重试路径**而非选择阶段。

## Evidence

复现命令（只读，不发上游请求）：

```bash
python3 scripts/codex_overload_report.py avoid      # 会话/模型/payload 控制变量
python3 scripts/codex_overload_report.py precursor  # 过载前的延迟变化
python3 scripts/codex_overload_report.py burst      # 爆发簇结构
python3 scripts/codex_overload_report.py warning    # 延迟预警规则的精确率/召回率
```

| 观测 | 数值 |
|---|---|
| 总会话 turn / 过载 | 20159 / 190 = **0.94%** |
| path1 HTTP 503（首包前失败） | **119**（首 turn 占 20） |
| path2 HTTP 200 + SSE `response.failed` | **71**（后续 turn 占 69） |
| 同会话重试成功率（同窗口对照） | **19.6%**（9/46） |
| 新会话重试成功率（同窗口对照） | **65.7%**（23/35） |
| 提升 | **3.36×** |
| 2×2：会话(新/同) × 模型(换/同) | 新会话 71.4% / 73.3%；同会话 22.6% / 75.0%(n=4) |
| payload 大小与过载率（按 input token 分桶） | 0.00 / 0.71 / 0.68 / 0.52 / 0.88%（**平坦，非驱动因素**） |
| 模型效应（同窗口控制后） | astra 19.5% vs 其他 14.0% = 仅 **1.39×** |
| 过载前一 turn 的会话内归一化 TTFT | path1 mean **3.82×**，46.7% 样本 >2×（基线 8.2%） |
| 爆发簇 | 57 个，最大 **13 次 / 42 分钟** |
| 延迟预警规则（K=2.5） | 精确率 **18.3%**，召回 **10.0%** |
