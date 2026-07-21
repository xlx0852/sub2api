## Context

Sub2API 的 OpenAI/Codex 路径已经具备：

- 三层调度：`previous_response_id` → `session_hash` → `load_balance`
- 加权 LB（priority/load/TTFT/error/quota headroom/compact penalty）
- 429 via `x-codex-*`、401 temp unschedulable、403 连击
- sticky TTL、WS response_id 映射、first-output timeout（默认关）、compact keepalive

OpenCodex 作为本机参考实现，证明了另一组更硬的不变量在长会话与多账号池下更稳。本设计把这些不变量移植到多租户 Go 网关，而不是复制其文件状态机或本机注入。

## Goals / Non-Goals

### Goals

1. 上游结果按 class 更新健康与 sticky，避免 thrash 与凭证串池。
2. sticky 在“连续对话”和“配额均衡”之间可推理：钉死 + generation + 节制 re-eval + 新会话 lowest-usage。
3. 流式超时有唯一终态语义：`incomplete`，且 keepalive 不等于进度。
4. 行为可配置、可观测、可分阶段开启；默认路径对非 Codex 客户端尽量无感。

### Non-Goals

- 替换现有加权公式或重写 scheduler 架构。
- 本机 Codex catalog 注入 / history remap。
- hosted web_search loop、vision sidecar。
- 全协议 AdapterEvent 总线。
- 改变计费单价或余额模型。

## Decisions

### Decision 1: 单一 outcome-class 入口

所有 OpenAI 上游终态（HTTP status、connect error、timeout、stream stall）先映射到：

| Class | 典型来源 | 健康影响 | Sticky |
|---|---|---|---|
| `success` | 2xx 完成 | 清 consecutiveFailures；**保留**未过期 cooldown | 保留 |
| `caller` | 多数 4xx（非 401/403/429） | 不计健康 | 保留 |
| `credential` | 401/403、明确 auth 失效 | needs-reauth / temp or permanent unschedulable | **清除该账号** |
| `quota` | 429 | cooldown；不计 transient 连续失败阈值 | **清除该账号**，后续请求换号 |
| `transient` | 5xx、connect、timeout | 滑动窗口连续失败，达阈值 failover | 可达阈值后清除 |

实现落点：`ratelimit_service` 与 gateway response handling 共用一个分类函数，scheduler 只消费 class 结果，不直接解析杂乱 status。

### Decision 2: Cooldown 时间源优先级

```text
Retry-After (delta-seconds 或 HTTP-date)
  > Codex/reset headers (primary/secondary/window reset_at)
  > default_quota_cooldown (建议 60s)
```

- clamp 到合理上下界（例如 1s..24h，key 池可更短上限如 10m）。
- `success` 不清除仍在未来的 `cooldownUntil`。
- key 池冷却键是 **attempted key id/hash**，rotate 使用 CAS：若 live key 已不是 attempted，则不再二次 rotate。

### Decision 3: 凭证类隔离

调度请求携带 `RequiredCredentialClass`：

- `oauth_chatgpt`（Codex login / OAuth）
- `api_key`

规则：

- sticky 命中若 class 不匹配 → 视为未命中。
- failover `ExcludedIDs` 扩展之外，候选集先按 class 过滤。
- 不允许 “OAuth 失败自动落到同组 API Key” 或反向，除非管理员显式配置跨 class 路由（本 change 默认禁止，不实现跨 class 开关）。

### Decision 4: Sticky 记录形状

在现有 session/response sticky 值上扩展（Redis JSON 或并行字段）：

```text
{
  account_id,
  credential_class,
  credential_generation,
  created_at,
  last_used_at,
  last_reeval_at
}
```

- `credential_generation`：OAuth token version / refresh generation / API key material hash 版本；账号凭据轮换或 reauth 成功后递增。
- idle TTL：沿用/配置化（session 默认可与现网 1h 对齐，也可对 response_id 单独 TTL）。
- 容量：对本地 map 做 LRU 上限；Redis 靠 TTL。
- re-eval 间隔默认 60s；阈值默认 80（usage percent）；仅当存在 **严格更低** usage 的同 class 可调度账号时才换绑。

### Decision 5: Usage score

```text
usageScore = max(available window percents)
unknown windows only → 100 (hottest)
plan-specific window masking MAY drop irrelevant windows later
```

- 现网已有 5h + 7d headroom 字段；本 change 先用已有窗口，不强制引入 30d schema。
- 新会话（无 previous_response、无有效 session sticky）在 LB 层：先 lowest-usage，再套现有加权作为 tie-break。
- 全 unknown：稳定 hash(session 或 request salt) round-robin，避免全员钉在同一默认号。

### Decision 6: previous_response_id 全传输层

- WS 已有 response→account 映射；HTTP/SSE 成功响应后同样写入。
- 读取路径：schedule 前解析 body/header 中的 `previous_response_id`，命中则 layer=`previous_response_id`。
- 与 generation/class 校验同一套；过期或 generation mismatch 则下降到 session/LB。

### Decision 7: Dual-clock stall

两个时钟：

1. **client_idle_arm**：周期性 SSE comment 或 `response.heartbeat`（若客户端兼容）；只服务传输保活。
2. **upstream_stall**：仅在收到真实上游字节/可解析事件时重置。

超时动作（有序）：

1. 标记 terminal reason=`upstream_stall_timeout`
2. 关闭未完成 message/tool/reasoning item（协议允许范围内）
3. 写 `response.incomplete`（Responses）或等价 chat 终态
4. cancel 上游 context
5. 计费：按已计量 token capture/release（沿用现有 usage 路径）
6. outcome-class 记 `transient`（可配置是否计入连续失败）

与 first-output timeout 关系：

- first-output：从请求发起到**首个上游进度**的专用预算（可更短）。
- mid-stream stall：首个进度之后的静默预算。
- compact soft-timeout：仍由 compact change 管；通用 stall 不抢 compact 的 “可换号重试一次” 语义，除非尚未 commit 任何下游事件且 compact 路径显式复用。

默认值建议：

- `openai_stream_stall_timeout_seconds=300`（开启）
- `openai_first_output_timeout_seconds` 保持可配；若为 0，不影响 mid-stream stall
- 高 effort 可覆盖更长 stall（可选后续）

### Decision 8: 发布方式

三阶段：

1. **observe**：只打 class/reeval/stall 指标与日志，不改变选择与终态。
2. **group flag**：管理分组或 setting 开启 enforce。
3. **default on**：OpenAI/Codex 路径默认 enforce；保留 kill switch。

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| re-eval 换号破坏 prompt cache | 仅阈值以上 + 间隔门闩 + 同 class；指标监控 cache miss |
| incomplete 终态被老客户端当错误 | 文档 + 与 OpenAI 语义对齐；可对非 Responses 客户端保持旧错误码 |
| generation 频繁变化导致 sticky 失效 | generation 仅在真实凭据轮换时递增，不在每次 refresh 空转递增 |
| lowest-usage 与加权 LB 冲突 | 新会话 primary key 用 usage，加权只做 tie-break；老会话仍 sticky 优先 |
| 默认开启 stall 改变长思考任务 | 默认 300s 对齐常见客户端 idle；高 effort 可加长；提供配置 |

## Migration Plan

1. 落地 class 枚举与分类纯函数 + 单测（无行为变化）。
2. sticky schema 扩展：读写双兼容（缺 generation 视为 0，命中后 backfill）。
3. observe 模式指标：`outcome_class_*`、`sticky_reeval_*`、`stall_timeout_*`。
4. 分组开启 credential isolation + cooldown 新优先级。
5. 开启 sticky re-eval + new-session lowest-usage。
6. 开启 dual-clock stall → incomplete。
7. 全量默认 on，保留 setting 关闭。

## Open Questions

1. stall 超时是否计入账号 transient 连续失败并触发换号重试，还是只结束当前请求？（建议：默认只结束当前请求，不自动换号重放，避免重复副作用；compact 已有专门 soft-timeout retry 除外。）
2. session sticky TTL 是否从 1h 提高到更接近长会话的 24h，还是保持 1h 仅靠 `previous_response_id` 续命？
3. heartbeat 事件名：对 Codex 用 `response.heartbeat`，对通用 SSE 是否只用 comment keepalive？
