## Context

`server_is_overloaded` 是唯一能触发客户端 `Selected model is at capacity. Please try a different model.` 的上游错误码，两条到达通道：

- **path1**：HTTP 503 + body `{"error":{"code":"server_is_overloaded"}}`（`codex-api/src/api_bridge.rs:76-81`）；WS 握手被拒时经 `map_ws_error`（`responses_websocket.rs:551`）也归入此类。
- **path2**：HTTP 200 + SSE 流内 `response.failed` 事件（`codex-api/src/sse/responses.rs:460`，判定函数 `is_server_overloaded_error` @745）。

客户端把该错误视为**终态**（`protocol/src/error.rs` 的 `retry_delay()` 返回 `None`），因此每次重试都是用户手动发起的新抽签。

回溯 20159 个 turn / 190 次过载（`scripts/codex_overload_report.py`）得到的核心事实：

1. 过载是**瞬时抽签**：同一会话 30 秒内交替成败；与 IP、账号、票据 TTL、payload 大小均无关。
2. 过载**黏着在会话标识上**：同会话重试 19.6%，新会话重试 65.7%（同 30 分钟窗口对照，3.36×）。
3. 过载**爆发式聚集**：57 个爆发簇，最大 13 次 / 42 分钟。
4. 上游**不提供前兆**：`/codex/models` 是静态目录；响应头只有配额（`x-codex-primary-used-percent`）；无 `warning` 事件；延迟预警精确率仅 18.3%。

## Goals / Non-Goals

- Goals: 让"可安全重放的过载"真正落在高成功率那一档；消除网关自身造成的路由钉死；保留计费与流式一致性；保真可观测性。
- Non-Goals: 不绕过 `codex_cli_only`；不做换模型降级；不改并发配额；不引入延迟预警；不轮换 `session_id`。

## Decisions

### Decision 1: 轮换派生的 `thread-id`，固定 `session_id`

- **做法**：`resolveConvergedThreadID(account, clientSessionID, attempt, epoch)`。
  - `attempt<=0 && epoch<=0` 时产出与现状**逐字节一致**（种子 `codex:thread-id:v1:<acc>:<sess>`）；
  - `attempt>0` 时 `v1:<acc>:<sess>:<attempt>`；
  - `epoch>0` 时换种子版本 `v2:<acc>:<sess>:<epoch>[:<attempt>]`，避免 epoch 与 attempt 数值碰撞派生出同值。
- **理由**：实测同会话重试 19.6% vs 新会话 65.7%。`thread-id`/`session-id` 即客户端的 `prompt_cache_key`，决定上游路由与缓存亲和。
- **Alternative considered**：轮换 `session_id`。**否决** —— `openai_codex_fingerprint.go:18` 明确 `session_id` 是刻意收敛为账号级常量以控制上游“设备数/会话数”判定的。
- **Alternative considered**：换模型。**否决** —— 同 30 分钟窗口内对照仅 1.39×，属时间混淆；2×2 分解显示新会话下不换模型也是 71-73%。

### Decision 1b: 跨请求需要独立的过载 epoch（而非复用请求内轮次）

- **问题**：`attempt` 存在 gin context（单个 HTTP 请求）内。客户端跨请求重试时轮次归零，派生回**同一个** `thread-id` —— 而 3.36× 的实测收益恰恰来自跨请求场景。
- **做法**：按 `(account.ID, 客户端 session-id)` 维护过载 epoch。过载时 `epoch++`；窗口（复用 30 分钟的爆发窗口）内派生使用 `v2` 种子；窗口外回到 `v1`。
- **键与内存**：键为 `sha256(clientSessionID)[:8]` 前缀 + accountID，**不保留原始 session-id**；上限 4096 条，超限清理过期项、仍超限则整体重置（epoch 只是路由启发式，丢失只意味着回到确定性派生）；另用 `atomic` 计数做「从未过载」的免锁快路径。
- **epoch 单调不回退**：窗口过期只用“是否生效”判定，不把计数器归零，避免同一会话重新拿到用过的派生值。
- **代价与权衡**：客户端自身续聊在该窗口内失去 thread 亲和（丢 prompt cache）。接受理由：窗口内正是上游降载期，而一次失败重试的代价（整轮重发全部历史，`store=false`）远大于一次 cache miss；且仅对**发生过过载的那个账号+会话**生效。
- **Alternative considered**：只在请求内轮换，跨请求交给客户端换会话。**否决** —— 那等于要求用户放弃当前对话，产品上不可接受，也是本变更要消除的负担。

### Decision 1c: 过载副作用收敛到单一入口

- **做法**：`HandleOpenAICodexOverload(account, overloadPath, clientSessionID)` 同时做「记爆发窗口 + 推进 epoch + 清票」。
- **理由**：三者必须同步，分散在多个调用点会漂移；且它们的前置条件不同（清票需要票据复用开关，其余不需要），放在一处才好表达。
- **附带**：WS ingress session 的指纹快照按 `(attempt, epoch)` 双维度校验刷新，否则头（从 context 重新解析）与体（`client_metadata`）会携带不同的 `thread-id`。

### Decision 2: 首次过载即丢弃被钉住的 `x-codex-turn-state`

- **做法**：`server_is_overloaded` 走独立分支直接丢弃该账号/池的票，绕过 `openAICodexTurnStateUpstream5xxInvalidateThreshold = 3`（`openai_codex_turn_state.go:473`）与 2 分钟冷却（`:516`）。
- **理由**：票是路由提示而非绑定凭证（292 字节票 ↔ astra 池、312 字节 ↔ luna 池，且持 292 票期间上游停止发新票）。过载是瞬时的，等 3 次意味着前 2 次白白钉死在同一个池。
- **Alternative considered**：把阈值常量从 3 降到 1。**否决为唯一手段** —— 会同时影响非过载的 5xx 语义（如真实上游故障），过载应有独立分支。

### Decision 3: 流内过载复用同一恢复路径

- **做法**：把 `response.failed` + `error.code == "server_is_overloaded"` 与 HTTP 503 归入同一个可恢复判定，仍受"已输出可见内容不重放"约束（复用 `writerSizeBeforeForward` 语义）。
- **理由**：path2 占 71/190，且长尾严重（实测最长 86 分钟后才 `response.failed`）。当前若只在流开始前判定，这部分会全部透传给用户。

### Decision 4: 爆发窗口预算

- **做法**：按账号组记录过载时间戳，窗口内（默认 30 分钟量级）提高重试/轮换上限，窗口过期回落。
- **理由**：爆发簇实测最大 13 次 / 42 分钟；单次过载后 10-40 分钟是明确的高危窗口。

### Decision 5: 不做延迟预警

- **理由**：滚动 30 分钟 p50 TTFT > K×基线 的规则，在 K=2.5 时精确率 18.3%、召回 10.0%；K 降低则误报爆炸（K=1.2 时 1655 次预警仅 49 次命中）。延迟升高有太多非容量原因（长 agentic 任务、工具调用、大上下文）。

## Risks / Trade-offs

- **prompt cache miss** → 轮换 `thread-id` 会使重试失去上下文缓存。缓解：仅对"未产生可见输出"的重试轮换，此时本来就没有成功产出；且轮换只发生在失败之后。
- **上游 thread 数观测变化** → `thread-id` 维度本就随客户端会话变化，非账号级敏感字段；`session_id` 保持收敛。缓解：灰度期对比账号健康指标。
- **流内过载的重放成本** → path2 已消耗上下文与工具调用，重放代价高。缓解：沿用既有预算上限，并对"已输出可见内容"一律不重放。
- **A/B 未做前的因果性缺口** → 上述 3.36× 是回溯相关性（虽经会话内归一化、爆发深度匹配、同窗口匹配三层控制）。缓解：任务 4.1-4.3 要求灰度 A/B 才能确认，验收阈值明确。
- **多实例预算状态不共享** → 复用现有账号调度状态，不新增 DB 迁移；若观测到跨实例同步撞击，再单独评估 Redis 协调。

## Migration Plan

1. 补充行为与场景单测（任务 3.1-3.5），重点防 attempt=0 的回归。
2. 本地完成定向 Go 测试与构建（3.6）与 OpenSpec 严格校验（3.7）。
3. `test` 分组灰度：A/B 两组对比重试成功率与用户可见 503（4.1-4.3）。
4. 指标确认后进入生产分组灰度；保留旧容器以便回滚（4.4）。

## Open Questions

- 爆发窗口的默认时长是取实测上界（42 分钟）还是保守取 30 分钟？
- `attempt_index` 是否需要在跨账号切换时也参与派生，还是仅在"同账号重试"时递增？（当前倾向后者：跨账号切换本身已通过 `account.ID` 改变 `thread-id`。）
