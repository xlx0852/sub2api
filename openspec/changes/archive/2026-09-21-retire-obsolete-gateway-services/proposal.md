# Change: Retire Obsolete Gateway Services and Dead Background Workers

## Why

1. **Batch Image (批量生图) 僵尸进程**: 批量生图公开路由早已在 `routes/gateway.go` 注释下线（`// 批量生图（Gemini 系）已随 gemini 供给下线。`），生产 PostgreSQL 中 `batch_image_jobs`、`batch_image_items`、`batch_image_events` 数据量均为 0。然而 `wire.go` 仍然在系统启动时无意义运行 `BatchImageWorkerRuntime` (`runtime.Start()`) 和 `BatchImageCleanupService` (`svc.Start()`)，常驻 Goroutine 并在后台持续轮询 Redis 和数据库，浪费系统资源。`BatchImageHandler` 也处于孤立未挂载状态。
2. **Gemini 原生直连/兼容网关已退役**: 官方 Gemini 原生供给已下线，全部 Gemini 流量已统一由 `AntigravityGatewayService` 承载，生产环境 0 个原生 Gemini 账号。底层驱动客户端 `gemini_drive_client.go`、`gemini_oauth_client.go`、`geminicli_codeassist_client.go` 均已打桩返回 `retired`，但 `GeminiMessagesCompatService` (2,527 行)、`GeminiChatCompletionsCompatService` (648 行) 及其配套 pipeline 仍被 Wire 完整装配并在 Handler 中保留死分支。
3. **Claude OAuth CLI 拟态层无业务需求**: 用户明确确认没有 OAuth 需求，生产环境中 Anthropic 账号 100% 为 APIKey。`gateway_claude_oauth_body.go` 中 1,200 余行专门用于伪装 Claude Code CLI 终端、生成虚拟终端指令与构造 billing attribution 块的拟态代码完全闲置。

## What Changes

- **Batch Image 僵尸组件下线**:
  - 从 `backend/internal/service/wire.go` 中移除 `ProvideBatchImageCleanupService` 与 `ProvideBatchImageWorkerRuntime` 的后台启动和 Wire ProviderSet。
  - 移除 `NewBatchImagePublicService`, `NewBatchImageDownloadService`, `ProvideBatchImageModelPricingResolver` 的 Wire 依赖注入。
  - 移除孤立无挂载的 `backend/internal/handler/batch_image_handler.go`。
  - 停止后台无意义的 Goroutine、Redis 队列轮询和定时清理任务。
- **Gemini 原生直连网关退役与解绑**:
  - `GatewayHandler` 解除对 `GeminiMessagesCompatService` 的依赖；对于 Gemini 协议请求，统一收敛至 `AntigravityGatewayService`。若遇到非 Antigravity 账号，直接返回 410/400 错误。
  - 从 `wire.go` 移除 `NewGeminiMessagesCompatService`, `ProvideGeminiTokenProvider`, `NewGeminiOAuthService`, `NewGeminiQuotaService` 等组件的启动或 Wire 暴露，彻底释放 ~4,500 行退役死代码的装配。
- **Claude OAuth CLI 伪装拟态层精简**:
  - 精简 `gateway_claude_oauth_body.go`，剥离虚假 Claude Code CLI 终端指令生成、虚假 billing attribution 块注入等无用逻辑，保留 APIKey 转发所需的通用请求体处理。

## Impact

- Affected specs: `architecture-governance`
- Affected code:
  - `backend/internal/service/wire.go`
  - `backend/internal/service/wire_gen.go`
  - `backend/internal/handler/batch_image_handler.go`
  - `backend/internal/handler/gateway_handler.go`
  - `backend/internal/handler/gemini_v1beta_handler.go`
  - `backend/internal/service/gemini_messages_compat_service.go`
  - `backend/internal/service/gateway_claude_oauth_body.go`
