# 官方 Codex CLI 性能优化研究（openai/codex，2026-09-25）

> 保存位置说明：`docs/` 在本仓库被 .gitignore 忽略，故本研究报告放在 `openspec/` 下（与 `CLEANUP-*.md`、`DATAFIX-*.md` 同类）。

对照源码：`openai/codex` 默认分支（`d7b07d4`），`codex-rs/` 工作区，重点文件如下。目的是回答「同一套 Responses/Codex 协议，官方客户端在哪些地方省时间/省字节」，并映射到我们网关可采纳的项。

## 1. 传输层：WebSocket 优先 + 连接复用 + 预热

- `core/src/client.rs:1-25`（模块注释）：`ModelClient` 贯穿整个会话，`ModelClientSession` 每 turn 一个，**懒建立 Responses WebSocket 并对同 turn 的多次请求复用**；缓存「上一次完整请求」用于增量发送；认证归属变化时丢弃连接、增量状态与路由。
- 预热（prewarm）：v2 专有的 `response.create` + `generate=false`，等它完成后再发真正的请求，使下一次请求可以复用**同一条连接**和 `previous_response_id`；预热失败则退回普通重试逻辑（注释里写明这是「retry-budget tradeoff」）。
- 粘性路由：`x-codex-turn-state`（`X_CODEX_TURN_STATE_HEADER`）由服务端下发，客户端在同一 turn 内原样带回，保证同一 turn 命中同一后端。
- 连接特性：`codex-rs/http-client/src/client_builder.rs` 暴露 `http2_prior_knowledge`、`connect_timeout`，并按路由（代理/直连）维护独立客户端池（`route_aware_client_pool.rs`）。

**我们已有**：WS 模式（`openai_ws_mode`、ingress/v2 两条链路）、turn-state 复用（`openai_codex_turn_state.go`）、连接复用指标（`ws_conn_reused`）。

**可采纳**：
1. **WS 预热**（我们目前是「客户端连上来 → 首个请求才建立上游 WS」）；对 ingress WS 模式，可在客户端握手后立刻向上游建立连接并发送 `generate=false` 预热，让首轮请求省掉一次握手+路由协商。
2. **上游 WS 连接池化**：同一账号多个客户端会话共用上游连接（官方是每 session 一条；我们可做每账号 N 条 + 亲和）。

## 2. 请求体：增量发送 + 15MiB 硬上限 + 元数据卸载

- **增量发送**（`core/src/client.rs:1396-1435` `get_incremental_items`）：当非 input 字段与上次一致、且本次 `input` 是上次 `input + 服务端返回输出` 的扩展时，只发 `previous_response_id` + **新增条目**；否则退回全量。判等重要到「忽略内部元数据的逐条比较」。
- **硬上限 + 卸载**（`core/src/client_tool_metadata.rs`）：出站消息超过 `MAX_RESPONSE_MESSAGE_BYTES = 15 * 1024 * 1024` 时，按预算卸载**可选工具元数据**（`bound_executed_tool_calls_for_message`：先卸 result metadata，再截断 call arguments，保留资源访问证据与用户内容），并记录 shedding 指标；序列化失败才走原有错误路径。
- **压缩**：`core/src/client.rs:1627-1636` —— 仅当 `enable_request_compression && auth.uses_codex_backend() && provider.is_openai()` 时对请求体 zstd 压缩（无尺寸阈值）。

**我们已有**：OAuth/Codex 路径 zstd；第三方中转 gzip（今天上线，出站降到 29.5%）；`strict_incremental_ok` 之类的增量**校验**。

**可采纳**（按收益排序）：
1. **网关侧增量转发**：HTTP 路径目前整包转发（实测 body p50 5.8–8.9MiB、max 51MiB）。若客户端带 `previous_response_id`，我们可以只把 delta 转发给上游（需保存上次 input 指纹，风险中等，收益最大：字节数可降一个数量级）。
2. **15MiB 出站上限 + 元数据卸载**：与官方同口径，避免 51MiB 级请求把写耗时/中转接收拉满。
3. **gzip 白名单扩容**：把已验证支持压缩的上游逐个加入（当前仅 o10.top）。

## 3. 上下文与压缩（compaction）

- 自动压缩阈值 = **上下文窗口的 90%**（`protocol/src/openai_models.rs:525-535`），另有 `effective_context_window_percent = 95`（为系统提示、工具开销与输出预留余量，`openai_models.rs:389-391`）。
- 压缩有本地/远端两代（`core/src/compact*.rs`，含 `compact_remote_v2` 与 image budget、token budget 变体），并在 pre-sampling 与 turn-end 两个时机触发（`core/src/session/turn.rs:182-190, 710-720`）。

**对我们的意义**：压缩由客户端驱动，我们不改语义；但它是「body 5–9MiB」的来源之一，也说明**增量发送/状态复用**才是把体量真正压下去的手段。

## 4. 计时与可观测：向上游索取引擎侧耗时

- 客户端可发 `x-responsesapi-include-timing-metrics: true`（`core/src/client.rs:170, 1335-1340`），上游在 WS 事件里返回 `timing_metrics`：
  - `responses_duration_excl_engine_and_client_tool_time_ms`（API 侧开销）
  - `engine_service_total_ms`（推理总时长）
  - `engine_iapi_ttft_total_ms` / `engine_service_ttft_total_ms`（引擎/服务首 token）
  - `engine_iapi_tbt_across_engine_calls_ms`（token 间隔）
  （消费点：`otel/src/events/session_telemetry.rs:70-75, 1309-1360`）

**可采纳**（与今天上线的「请求耗时详情」天然契合）：我们在调用 Codex 后端时带上该头，把 `engine_service_ttft_total_ms`（模型真实首 token）与 `engine_..._total_ms`、`responses_duration_excl_engine...`（上游 API 开销）落进 `first_output_stages` 并在瀑布里单开一条「上游引擎」泳道。这样能把当前 12s 级 TTFT 拆成「引擎推理 vs 排队/网络」，直接回答「还能不能优化」。

## 5. 重试与失败切换

- `core/src/responses_retry.rs`：流式与连接失败**分开预算**；连接重试指数退避 5s → 60s；尊重服务端 `Retry-After`；重试建议按 `turn_id` 归属，避免复用的会话套用旧建议。

**我们已有**：失败切换/多账号重试与冷却。**可采纳**：把「连接类失败」与「流中断」分开计数与退避（避免流中断触发账号级冷却），并显式遵守 `Retry-After`。

## 6. 其余细节（参考价值）

- `store: false` + `previous_response_id` 的会话复用模型（不依赖服务端存储）。
- `prompt_cache_key` = 稳定会话标识（子代理用 `source:parent_thread_id`），随后又用它作为 Responses 会话头来获得缓存亲和（`core/src/client.rs:585-607`）——缓存命中率的根源在于「键要稳定」。
- 工具输出与 rollout 截断（`core/src/thread_rollout_truncation.rs`）等客户端侧省 token 手段。

## 结论（对网关的优先级）

1. **上游引擎耗时采集**（第 4 节）：改动小、风险低、立刻把 TTFT 归因说清楚——最快能落地。
2. **15MiB 出站上限 + 工具元数据卸载**（第 2 节）：与官方同口径，直接削掉 51MiB 级请求的尾部。
3. **WS 预热 / 上游连接池化**（第 1 节）：降低 WS 模式首轮 TTFT。
4. **网关侧增量转发**（第 2 节）：收益最大、风险也最大，建议先做「能判定就增量、判定不了就全量」的保守版。
5. **连接类失败与流中断分开退避 + Retry-After**（第 5 节）：降低无效账号冷却与重试放大。
