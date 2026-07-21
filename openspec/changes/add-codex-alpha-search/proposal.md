# Change: Add Codex Alpha Search Endpoint

## Why

Codex 新版客户端会把独立网页搜索发送到 `alpha/search`，而当前网关只注册 Responses、Chat Completions、Embeddings 等通用端点。请求因此会在本地路由层返回 404，无法复用现有 OpenAI OAuth/API Key 账号、调度、并发和故障切换能力。

上游提交 `52071d391` 已实现该独立端点，但本地分支包含额外的 Codex compact、WS、Grok 和用量记录逻辑，接入时必须维持职责边界，并避免把 alpha schema 固化进通用 Responses 类型。

## What Changes

- 注册 `/v1/alpha/search`、`/alpha/search` 和 `/backend-api/codex/alpha/search`
- 仅允许 OpenAI 分组使用独立搜索端点
- OAuth 账号转发到 ChatGPT Codex alpha/search，API Key 账号转发到官方或自定义 OpenAI alpha/search
- 透明保留搜索请求、查询参数和响应体，仅应用模型映射与服务端鉴权
- 复用现有用户/账号并发、计费资格检查、粘性调度、账号健康反馈和 HTTP failover
- 增加端点归一化、路由注册、OAuth/API Key 请求和 failover 回归测试

## Impact

- Affected specs: `codex-alpha-search`
- Affected code: OpenAI endpoint normalization, gateway route registration, OpenAI gateway handler, standalone search forwarding service and tests
- Preserved behavior: Responses/compact/WS semantics, Grok routes, non-OpenAI groups, database schema and frontend
