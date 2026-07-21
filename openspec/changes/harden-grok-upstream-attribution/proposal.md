# Change: 强化 Grok 上游重试与身份归因

## Why

当前 Grok 网关主要依据 HTTP 状态码决定同账号重试、账号切换和健康惩罚，尚未使用 xAI 的显式重试抑制信号，也缺少跨协议一致的服务端请求身份与 401 凭据新旧归因。这会造成不必要的重试放大，并增加上游问题、缓存行为和 Token 轮换故障的定位成本。

## What Changes

- 识别上游 `x-should-retry: false`，禁止当前失败触发同账号重试、账号切换和账号健康惩罚，同时保持原始错误响应兼容性。
- 为 Grok Responses、原生 Chat Completions 和 Chat-via-Responses 注入服务端派生、租户隔离的 request/session/attempt 身份头。
- 对 Grok OAuth 401 进行 `stale`、`current`、`unknown` 分类，仅记录不可逆 Token 指纹与版本；陈旧 Token 失败不惩罚账号。
- 补充跨协议、重试顺序、Header 隔离、Token 轮换和敏感信息防泄露回归测试。

## Impact

- Affected specs: `grok-safe-same-account-retry`, `grok-request-identity`, `grok-token-attribution`
- Affected code:
  - `backend/internal/handler/grok_same_account_retry.go`
  - `backend/internal/handler/openai_gateway_handler.go`
  - `backend/internal/handler/openai_chat_completions.go`
  - `backend/internal/service/openai_gateway_grok*.go`
  - `backend/internal/service/openai_gateway_chat_completions_raw.go`
  - `backend/internal/service/grok_token_provider.go`
  - Grok forwarding and handler regression tests
- No database migration and no client-visible API schema change.
