## 1. Batch Image 僵尸服务清理
- [x] 1.1 从 `backend/internal/service/wire.go` 移除 `ProvideBatchImageCleanupService`、`ProvideBatchImageWorkerRuntime`、`ProvideBatchImageModelPricingResolver`、`NewBatchImagePublicService`、`NewBatchImageDownloadService` 的注册与启动调用
- [x] 1.2 移除孤立且未挂载的 `backend/internal/handler/batch_image_handler.go`
- [x] 1.3 运行 `wire` 重新生成 `wire_gen.go`，确保编译通过

## 2. Gemini 原生直连/兼容网关退役与解绑
- [x] 2.1 修改 `backend/internal/handler/gateway_handler.go`，移除 `geminiCompatService` 依赖，将非 Antigravity 的直接 Gemini 转发分支安全废除（直接返回错误）
- [x] 2.2 修改 `backend/internal/handler/gemini_v1beta_handler.go`，移除 `geminiCompatService.ForwardNative` 分支，统一由 `antigravityGatewayService.ForwardGemini` 提供服务，非 Antigravity 账号返回 410/400
- [x] 2.3 从 `backend/internal/service/wire.go` 中移除 `NewGeminiMessagesCompatService` 注入
- [x] 2.4 重新运行 `wire` 生成 `wire_gen.go`，并确保相关网关测试正常

## 3. Claude OAuth 伪装与拟态层精简
- [x] 3.1 审查 `backend/internal/service/gateway_claude_oauth_body.go`，移除冗余的虚假 Claude Code CLI 终端模拟、虚假计费块注入等死逻辑
- [x] 3.2 保持 Claude APIKey 正常透传与模型改写功能完好

## 4. 验证与回归测试
- [x] 4.1 运行完整的网关测试套件 (`go test ./backend/internal/handler/... ./backend/internal/service/...`)
- [x] 4.2 验证后台不再启动 batch image worker 轮询与 cleanup 轮询
