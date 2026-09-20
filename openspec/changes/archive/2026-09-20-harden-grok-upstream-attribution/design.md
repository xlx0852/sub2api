## Context

Sub2API 同时代理 Grok Responses、原生 Chat Completions 和 Chat-via-Responses，并在 handler 层编排同账号重试与跨账号切换。服务层当前在返回 `UpstreamFailoverError` 前可能执行账号惩罚，而 Token 获取接口只返回字符串，无法在后续 401 时准确判断实际发送的凭据是否已经陈旧。

## Goals / Non-Goals

- Goals:
  - 尊重 xAI 的显式不重试信号，避免重试放大和账号误伤
  - 为一次下游请求的全部 Grok 上游尝试提供稳定、脱敏、可关联的身份
  - 区分陈旧 Token 与当前 Token 被拒绝，并安全记录归因
  - 保持现有协议响应、并发槽位、计费和安全重试边界
- Non-Goals:
  - 扩大 Grok 媒体或 WebSocket 的自动重试范围
  - 在流式语义输出后重放请求
  - 记录 Token 原文或前缀
  - 改变通用 OpenAI 账号的 401 策略

## Decisions

- Decision: `x-should-retry` 只接受大小写不敏感、去空白后的精确 `false`
  - `true`、缺失和非法值不强制重试，仅沿用现有状态码策略
  - 显式 `false` 在服务层阻止 `UpstreamFailoverError` 和所有 Grok/OpenAI 账号惩罚副作用

- Decision: 身份头全部由服务端覆盖
  - 使用 Grok Build 公开协议头：`x-grok-req-id`、`x-grok-session-id`、`x-grok-turn-idx`、`x-grok-agent-id`、`x-grok-model-override`
  - `x-grok-conv-id` 继续使用现有按 API Key 与模型隔离的缓存身份
  - request ID 优先使用服务端生成的 client request ID，避免信任客户端自带 Grok 身份头
  - attempt/turn 在每次真实上游请求前递增，跨同账号重试和账号切换保持单调
  - 第一阶段不向媒体与辅助描述请求注入 attempt 语义

- Decision: 401 归因绑定“实际发送的 Token”
  - 增加 Grok 专用 Token 元数据结果，不改变共享 Token Cache 的值格式
  - 元数据包含凭据账号、版本、来源和截断 SHA-256 指纹，不包含原文
  - 401 时读取凭据账号最新状态；实际发送 Token 的指纹变化判定为 `stale`，指纹一致判定为 `current`，无法确认判定为 `unknown`
  - Token 版本仅在能证明实际发送 Token 与账号快照一致时记录，用于辅助诊断而不覆盖指纹结论
  - `stale` 不临时下线或 runtime block；`current` 与 `unknown` 保持现有惩罚和 failover 行为

- Decision: 服务分类、handler 编排的现有边界保持不变
  - 显式不重试信号在服务层完成副作用抑制
  - handler 仅消费可重试错误并维护请求级 attempt 计数

## Risks / Trade-offs

- 中间层错误注入 `x-should-retry: false` 会降低 failover 可用性
  - Mitigation: 只接受精确布尔文本，并通过 wire 测试验证 Header 来源
- 身份头可能增加上游关联面
  - Mitigation: 只发送服务端 UUID/哈希身份，不发送原始 API Key、用户 ID 或客户端 session 值
- Token 轮换和 401 之间存在并发窗口
  - Mitigation: 归因绑定实际发送指纹，并将无法可靠判断的情况归类为 `unknown`、保留现有保守惩罚
- shadow account 可能与凭据账号不同
  - Mitigation: 归因始终读取实际凭据所有者，而不是只比较被调度 shadow account

## Migration Plan

1. 先补分类、身份头和 Token 归因测试。
2. 实现服务端 helper 与 forwarding 接入。
3. 接入 handler attempt 计数并验证槽位和重试顺序。
4. 运行 affected packages、race、vet 和 server build。
5. 通过蓝绿部署验证；回滚只需切回上一二进制。
