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
- [ ] 4.4 补充运维说明：class 含义、sticky re-eval、stall 配置与排障

## Implementation notes

- Sticky invalidation uses process-local epoch bump on credential/quota outcomes + epoch side-channel in session/response cache.
- HTTP previous_response_id is accepted for local affinity; force_http still ignores it; upstream wire may still strip non-WSv2 previous_response_id.
- Full cross-node reverse index sticky clear is deferred; unschedulable/rate-limit paths continue to clear on next sticky hit.
