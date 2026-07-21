## Context

Codex alpha/search 是独立 HTTP JSON 端点，不属于普通 `/responses` turn，也不使用 Responses SSE 或 WebSocket 生命周期。其请求和响应 schema 仍在演进，网关若将其绑定到现有 Responses DTO，容易删除未知字段或错误应用 compact、tool、session 和 WS 转换。

当前网关已经具备 OpenAI OAuth/API Key 凭证解析、模型映射、账号选择、并发控制、上游 URL 校验、请求头清洗、配额快照和 failover。接入应复用这些横切能力，但保持 alpha/search 的 wire contract 独立。

## Goals / Non-Goals

- Goals:
  - 让 Codex 客户端通过三种兼容路径调用独立搜索
  - 支持 OpenAI OAuth、官方 API Key 和自定义 OpenAI API Key 上游
  - 未知请求/响应字段透明保留
  - 在上游限流、认证和临时错误时沿用现有账号 failover
  - 正确记录入站与实际上游 endpoint
- Non-Goals:
  - 不把 alpha/search 转换为 Responses、Chat Completions 或 WebSocket 请求
  - 不向 Grok、Anthropic、Gemini 或 Antigravity 分组开放该端点
  - 不定义或修改 alpha/search 的业务 schema
  - 不新增数据库、前端开关或独立计费价格

## Decisions

- Decision: 使用独立 handler 和 service 转发函数
  - handler 负责认证上下文、JSON/model 最小校验、计费资格、调度、并发和 failover 循环
  - service 负责模型映射、凭证、目标 URL、头部清洗、请求发送和响应透传

- Decision: 请求和响应保持透明
  - 请求体只在模型映射命中时替换 `model`
  - query 参数原样追加到目标 alpha/search URL
  - 返回状态码、Content-Type、允许的响应头和 JSON body 原样写回
  - 不对未知字段做白名单过滤

- Decision: 按账号类型选择独立上游
  - OAuth 固定使用 `https://chatgpt.com/backend-api/codex/alpha/search`
  - API Key 使用账号 base URL 拼接 `/v1/alpha/search`
  - 自定义 base URL 继续经过现有 SSRF/URL allowlist 校验

- Decision: alpha/search 不进入 Responses/WS 状态机
  - 搜索 ID 仅可作为本次账号选择的 fallback seed
  - 不创建 `previous_response_id`、turn state、WS transport affinity 或 compact mode
  - endpoint capability 暂复用 OpenAI HTTP 可用性筛选，避免引入数据库能力字段

## Risks / Trade-offs

- 风险: alpha schema 继续变化
  - Mitigation: 最小解析 `model`，其余字段透明透传
- 风险: 自定义 OpenAI 上游没有实现该端点
  - Mitigation: 透传明确的 4xx；对现有策略认定的可切换错误执行账号 failover
- 风险: 独立搜索没有稳定 token usage schema
  - Mitigation: 本变更只执行现有计费资格检查和配额响应头快照，不伪造 token 用量或价格
- 风险: 与当前未提交的 Grok Codex 翻译改动混合
  - Mitigation: 新增文件保持独立，共享 endpoint/routes 文件仅做最小补丁并在实现前复查工作区差异

## Migration Plan

1. 增加端点归一化、路由和转发失败测试
2. 接入独立 handler/service，不改 Responses、WS 或 Grok 转换链
3. 运行 handler、service、routes 和 server-entry 定向测试
4. 通过现有蓝绿流程部署；异常时回滚应用版本，无数据迁移
