## 1. Outcome-class 健康机

- [x] 1.1 定义 `UpstreamOutcomeClass` 与纯函数分类（status/connect/timeout/stall）
- [x] 1.2 429 cooldown：解析 `Retry-After` + 现有 Codex/reset 头 + 默认值；clamp 上下界
- [x] 1.3 `success` 保留未过期 cooldown；`quota` 不计 transient 连续失败阈值
- [x] 1.4 `credential`（401/403）清该账号 sticky，并标记 reauth/unschedulable
- [x] 1.5 caller 4xx 不更新连续失败健康计数
- [x] 1.6 API key 池：冷却 attempted key + CAS rotate，覆盖并发 429 单测
- [x] 1.7 调度候选与 failover 强制同 `RequiredCredentialClass`（OAuth/API Key 不互串）
- [x] 1.8 指标/日志：outcome class、cooldown until、credential class（无 PII）

## 2. Session affinity 与新会话用量路由

- [x] 2.1 sticky 值扩展 `credential_class` / `credential_generation` / `last_reeval_at`，兼容旧格式
- [x] 2.2 凭据轮换/reauth 成功时递增 generation，并使旧 affinity 失效
- [x] 2.3 sticky 命中路径增加间隔 re-eval；超阈值且存在更低 usage 同 class 账号时换绑
- [x] 2.4 usageScore：`max` 可见窗口，unknown=100；全 unknown 稳定 round-robin
- [x] 2.5 无 sticky 新会话 LB 主序改为 lowest-usage，现有加权作 tie-break
- [x] 2.6 HTTP/SSE 写入并读取 `previous_response_id`→account 映射（与 WS 一致校验）
- [ ] 2.7 idle TTL + 本地 LRU/容量上限；过期与 generation mismatch 降级到下一层
- [x] 2.8 单测：钉死、re-eval 换号、generation 失效、跨 class 拒绝、新会话 lowest-usage

## 3. Stream stall dual-clock

- [x] 3.1 抽象 stream clock：client keepalive vs upstream progress
- [x] 3.2 配置项 `openai_stream_stall_timeout_seconds`（建议默认 300）与 kill switch
- [x] 3.3 Responses/SSE：超时写 `response.incomplete` + reason，取消上游，禁止假 `completed`
- [x] 3.4 与 first-output timeout、compact keepalive 语义对齐（文档化时钟职责）
- [ ] 3.5 用量结算：stall 终态走现有 incomplete/partial usage 路径
- [x] 3.6 回归：长静默 incomplete、有进度不误杀、keepalive 不重置 stall、客户端取消不双写终态

## 4. 发布与验证

- [ ] 4.1 observe 模式：只记指标不改路由/终态
- [ ] 4.2 管理 setting / 分组 flag 分阶段 enforce
- [x] 4.3 跑 scheduler、ratelimit、WS sticky、HTTP responses stream 相关单测
- [x] 4.4 补充运维说明：class 含义、sticky re-eval、stall 配置与排障

## 5. 生产容量降载终止帧修复

- [x] 5.1 在客户端有效输出前，将紧邻的 `error` / `response.failed` 容量降载帧统一转为可同账号重试的请求级瞬时错误
- [x] 5.2 终止帧运维详情只保留错误码、类型、状态、response ID 与 token 计数，不持久化 instructions/output/tools 等客户内容
- [x] 5.3 同一上游响应的紧邻终止帧去重，避免错误看板双计数
- [x] 5.4 增加 transformed/passthrough 双链路回归与日志脱敏/去重单测

## Implementation notes

- 2026-08-25: 修复 OpenAI HTTP 200 SSE 先返回紧凑 `type=error` 、后返回完整 `response.failed` 时裸错误帧提前固化客户端响应、阻断 failover 的问题；`server_is_overloaded` / `slow_down` 在首个有效输出前现在会保留响应可重试性。同时将终止帧 ops detail 收缩为脱敏结构投影，并对同一 upstream request ID 去重。定向 streaming、unit 与 ops handler 回归通过；全量 service 套件仍有一条与本修复无关的旧断言失败（invalid-request 默认映射预期 502，当前实现返回 422）。API `0.1.340` 已以 backend-only 蓝绿方式直接发布到美国生产 `18261`，用户已明确要求跳过 staging；前端保持 `0.1.339`，API `0.1.339`/`18260` 作为回滚点且 `restart=no`。候选与公网健康/版本/未授权门禁通过，新容器 healthy、0 重启；切流后首个观察窗 158 个请求中 149 个 200、7 个 WS 101、1 个预期 401 和 1 个运维探测 403，5xx/503 为 0，panic/fatal 为 0。发布前备份位于 `/opt/sub2api/backups/pre-0.1.340-20260825T111132Z`。

- 2026-08-16: WS ingress usage 复用口径缺陷已修复：首次新建 lease 记 `false`，同一入站长会话的后续 turn 仅在沿用上一轮成功 `conn_id` 时记 `true`，failover 新连接仍记 `false`。定向/race 回归通过；API `0.1.323` 已以 backend-only 蓝绿方式发布到美国生产 18248，候选直连 WS 两轮 usage 实证 `false → true`，公网 HTTP/SSE/WS 成功且结算入库；`0.1.322`/18247 完整保留为独立回滚。TTFT 权重是全局 setting，未按 group 4 名义修改生产全局权重；4.1 仍保持未勾选。
- 2026-08-16: 4.1 的可观测基础已增加管理员只读、进程级 sticky legacy 快照，包含读回退/命中/双写计数、开关与 TTL 上限；observe 模式整体验收仍未完成，故 4.1 保持未勾选。
- 2026-08-16: 4.4 已新增 `docs/OPENAI_GATEWAY_OPERATIONS.md`，记录 outcome class、60s/80% sticky re-eval、每实例 legacy 快照与先停双写后停旧读的退出步骤，以及 first-output/stall/client-disconnect 排障边界；未修改配置默认值。
- 2026-08-16: 进程级 compatibility snapshot 随 API `0.1.322` 后端单独发布到美国生产 18247；未授权请求返回预期 401，证明路由存在且管理员边界生效。未使用管理员凭据读取快照，故 4.1 observe 仍保持未勾选。
- Sticky invalidation uses process-local epoch bump on credential/quota outcomes + epoch side-channel in session/response cache.
- HTTP previous_response_id is accepted for local affinity; force_http still ignores it; upstream wire may still strip non-WSv2 previous_response_id.
- Full cross-node reverse index sticky clear is deferred; unschedulable/rate-limit paths continue to clear on next sticky hit.
