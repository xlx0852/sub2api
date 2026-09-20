# Change: 增强 Grok 请求效率、额度调度与视频编辑

## Why

Grok 多轮请求会重复携带图片和大型工具输出，容易导致请求体膨胀、超时和额度浪费；同时现有调度只在额度耗尽时排除账号，没有利用已持久化的剩余额度做软均衡。现有媒体链路也缺少 xAI 已支持的视频编辑入口。

## What Changes

- 为 Grok Responses 与 Chat 请求增加默认保守、可配置的请求体优化：精确图片去重、历史工具输出截断、图片请求 `store=false` 和软硬体积预算。
- 在不改变粘性会话和硬耗尽排除语义的前提下，将新鲜 Grok billing/API quota 快照作为调度软权重。
- 增加 `POST /v1/videos/edits` 与 `/videos/edits`，复用现有 Grok 媒体审核、调度、并发、计费、failover 和任务账号粘连。
- 增加配置校验、指标日志及三类功能的回归测试。

## Impact

- Affected specs: `grok-payload-optimization`, `grok-quota-scheduling`, `grok-video-edits`
- Affected code:
  - `backend/internal/config`
  - `backend/internal/service/openai_gateway_grok*.go`
  - `backend/internal/service/openai_gateway_scheduling.go`
  - `backend/internal/repository/scheduler_cache.go`
  - `backend/internal/handler/grok_media.go`
  - `backend/internal/handler/endpoint.go`
  - `backend/internal/server/routes/gateway.go`
  - `backend/internal/pkg/xai`
  - `deploy/config.example.yaml`
- No database migration and no new external dependency.
