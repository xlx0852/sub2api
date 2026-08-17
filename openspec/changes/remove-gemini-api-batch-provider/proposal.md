# Change: 退役 Gemini API 批量生图 Provider

## Why

Gemini 账号建号和公开入口已退役，但 `gemini_api` 批量生图 Provider 仍在默认 registry 和后台 Worker 中生产装配，使系统继续携带一整套 Gemini Batch API HTTP 客户端、文件上传/下载和状态映射代码。

2026-08-16 对美国生产 PostgreSQL 做的只读审计显示 `batch_image_jobs`、`batch_image_items`、`batch_image_events` 均为 0 行，当前没有需要恢复、结算或清理的 Gemini 批图历史任务。因此可以在不改变 Vertex 批图和普通 Gemini/Antigravity 请求的前提下，独立退役该 Provider。

## What Changes

- **BREAKING**: `gemini_api` 不再是可用的批量生图 Provider；显式请求该值返回现有 unsupported-provider 错误。
- 从默认/配置 Provider registry 和自动选择顺序中移除 `gemini_api`，保留 `vertex`。
- 删除 Gemini Batch API 专用 Provider、HTTP client、JSONL 组装/状态映射代码及对应测试。
- 收紧前端 provider 类型与批图文档，不再展示 `gemini_api`。
- 将“所有环境的非终态 `gemini_api` 任务为 0”作为部署硬门禁；不使用 SQL 强制标记失败或绕过原有资金释放/结算语义。

## Impact

- Affected specs: `batch-image-provider-lifecycle` (新)
- Affected code:
  - `backend/internal/service/batch_image_provider_gemini.go`
  - `backend/internal/service/batch_image_provider.go`
  - `backend/internal/service/batch_image_public.go`
  - `backend/internal/domain/batch_image.go`
  - 批图 repository/service 中仅用于 Gemini 的 fixtures 和断言
  - `frontend/src/api/batchImage.ts`
  - `docs/BATCH_IMAGE_MVP.md`
- Preserved behavior:
  - Vertex 批图 Provider、通用 Worker/队列/结算/清理不变。
  - Gemini/Antigravity 普通 HTTP/SSE 网关路径不变。
  - 协议 I/O、计费价格、YAML/env 键名和默认部署形态不变；本 change 不删除通用 `batch_image.*` 配置。
