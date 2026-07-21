# Change: 加固 OpenAI 账号池失败分类、会话粘连与流式 stall 生命周期

## Why

对照本机代理 OpenCodex 的账号池与流式实现后，Sub2API 已有更重的三层调度、加权 LB 和 sticky，但仍有三类会直接导致生产抖动的语义缺口：

1. **上游失败分类不够硬**：caller 4xx / credential / quota / transient 混在一起时，容易 thrash、误伤健康号，或 OAuth 与 API Key 静默串池。
2. **会话粘连缺 generation 与节制 re-eval**：长会话该钉死，但钉死后配额变热不会有节制换号；凭证轮换后旧 sticky 可能继续命中；新会话也缺少显式 lowest-usage 选择。
3. **流式 stall 生命周期不完整**：客户端 keepalive 与上游真实进度未分离；first-output timeout 默认关闭；超时路径更像错误/failover，而不是稳定的 `response.incomplete` 终态。

这些都是网关内核不变量，不依赖本机 Codex 注入，且能直接落在现有 `openai_account_scheduler` / `ratelimit_service` / stream forward 上。

## What Changes

### A. Upstream outcome-class 健康机

- 引入统一 `UpstreamOutcomeClass`：`success` / `caller` / `credential` / `quota` / `transient`。
- 429 cooldown 优先级：`Retry-After` → Codex/reset 头 → 默认 cooldown；成功响应**不得**清掉未过期 cooldown。
- 401/403 credential：fail-closed 清该账号 sticky，标记 needs-reauth / temp unschedulable；**同一次 failover 不得跨 OAuth ↔ API Key 凭证类**。
- caller 4xx 不计入账号健康连续失败。
- API key 池 429：冷却 **attempted key**，CAS 轮换到下一把未冷却 key，避免并发 429 惩罚刚换上的 key。

### B. Session affinity 语义升级

- sticky 记录绑定 `credential_generation`（或等价 token/key version）；generation 变化则 affinity 失效。
- 已有 session/response sticky 命中后，按配置间隔做配额 re-eval；超过阈值且存在更凉的同凭证类账号时，允许节制换号。
- 无 sticky 的新会话在 load-balance 层优先 lowest-usage（`max` 可见窗口用量；unknown=最热）；全 unknown 时按稳定顺序 round-robin，避免死钉。
- `previous_response_id` 亲和补齐 HTTP/SSE 路径（不仅限 WS），并与 session sticky / generation 规则一致。
- sticky 条目保留 idle TTL 与容量上限（LRU），防止无限膨胀。

### C. Stream stall dual-clock → incomplete

- Responses/SSE（及等价 chat 流，若共用桥）区分：
  - **client keepalive**：重武装客户端/代理 idle，不计入上游进度；
  - **upstream progress**：真实输出/事件才重置 stall 时钟。
- 配置化 stall 超时（建议默认开启，例如 300s，可按 effort/端点覆盖）。
- 超时后：关闭未完成输出项 → 向客户端写 `response.incomplete`（含 reason，如 `upstream_stall_timeout`）→ cancel 上游 → 尽量完成用量结算。
- 截断流**不得**标成 `completed`。
- 与现有 first-output timeout / compact keepalive 合并语义，避免多套互相打架的时钟。

## Impact

- Affected specs:
  - `openai-upstream-outcome-health`（新）
  - `openai-session-affinity-routing`（新）
  - `openai-stream-stall-lifecycle`（新）
- Affected code（实现阶段，待批准后）:
  - `backend/internal/service/ratelimit_service.go`
  - `backend/internal/service/openai_account_scheduler.go`
  - `backend/internal/service/openai_gateway_scheduling.go`
  - `backend/internal/service/openai_sticky_compat.go` / WS state store / HTTP sticky store
  - `backend/internal/service/openai_first_output_timeout.go`
  - `backend/internal/service/openai_gateway_forward.go` / response handling / compact keepalive
  - 相关 unit / WS / stream 回归测试
- Compatibility:
  - 非 OpenAI 平台默认不变。
  - 现有三层调度顺序保留：`previous_response_id` → `session_hash` → `load_balance`。
  - 行为变化主要在失败恢复、粘连换号阈值、流超时终态；需 feature flag / 分阶段默认开启。
- 明确 **Non-Goals**：
  - 不引入本机 Codex/`~/.codex` 注入 companion。
  - 不重写整套加权 LB 公式。
  - 不做 web_search 多轮 sidecar / vision sidecar（后续独立 change）。
  - 不做完整 AdapterEvent 总线大重构。

## Related Work

- 与 `update-codex-compact-health-routing` 互补：compact 有自己的 soft-timeout/SSE bridge；本 change 覆盖通用 Responses 流 stall 与账号池不变量。
- 与 `add-ws-account-health-metrics` 互补：后者偏观测与 WS 连接惩罚；本 change 偏 outcome-class 与 sticky 语义。
- 灵感来源（本机参考实现，非依赖）：OpenCodex `codex/routing.ts`、`codex/quota.ts`、`bridge.ts`、`providers/key-failover.ts`。
