# Change: Add Grok xAI WebSocket Passthrough

> Superseded by `remove-grok-websocket-ingress`. Grok text traffic now uses
> HTTP Responses/SSE only, and this passthrough capability must not be enabled.

## Why

当前 Grok 账号在 Codex WebSocket 场景中被强制走 HTTP bridge，上游会话、`previous_response_id` 和连接复用语义不够接近 Grok 官方客户端。`grok-composer-2.5-fast` 等 Composer 类模型对会话连续性更敏感，现有 HTTP bridge 容易出现稳定性和长尾延迟问题。

## What Changes

- 为 Grok/xAI OAuth 账号新增原生 WebSocket passthrough 上游通道
- 仅在 Codex/Responses WebSocket 场景和账号/分组显式启用时优先使用 Grok WS
- 保留现有 Grok HTTP bridge，作为非 WS、媒体请求、禁用 WS 或 WS 失败后的 fallback
- 为 Grok WS 增加请求头、body builder、`previous_response_id` 映射、session 级连接复用和失败剔除
- 将 Grok WS 连接状态和失败原因接入现有 WS 指标，便于区分连接慢、生成慢和请求体过大
- 补充 sync、stream、Codex WS、fallback 和响应 ID 映射回归测试

## Impact

- Affected specs: `grok-xai-ws-passthrough`
- Affected code: Grok gateway、OpenAI WebSocket forwarder、WS mode routing、Grok OAuth headers、usage/WS metrics、后台账号性能展示
