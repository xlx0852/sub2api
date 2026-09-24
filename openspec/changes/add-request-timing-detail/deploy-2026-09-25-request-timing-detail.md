# 请求耗时详情分段 — 生产发布

日期：2026-09-25 06:30 JST（05:30 +08）
代码：`61e5f051f`（前端 + 后端首版）、`ba5c35d19`、`46cf8ce51`（后端补埋点）
change：`add-request-timing-detail`
生产机：74.82.195.84

## 1. 产物

| 层 | 内容 |
| --- | --- |
| 前端 | release `/opt/sub2api/frontend/releases/0.1.391-61e5f051f`（171 文件，聚合 `54635e8729e9827cc0f3f976e6cfdbb44488ede3a5cb339adaf6f2f450677bab`，`index.html` = `799665e4c277389b513b2ca0bd11c1f69b8e712037e209241a7f8fe6f24a2447`）；`current` → 该版本，`rollback` → `0.1.391-01a904fb3` |
| 后端 | 最终运行二进制 `sha256=7260ed5cc0357b32f0a8da22539e722a73e9d14b3b702e613c97d7763becb61d`（由 `46cf8ce51` 构建）；备份 `sub2api.bak-20260925-timing2`（`a49fb479…`，即首版 `61e5f051f`） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，`Up (healthy)`、`RestartCount=0` |

## 2. 线上核验（真实流式请求）

`POST https://code.sicts.shop/v1/responses`（`gpt-5.6-luna`，`stream=true`，客户端 key）→ 200，落库 `first_output_stages`：

```json
{
  "outcome": "success", "body_bytes": 108, "body_first_wait_ms": 5, "body_receive_ms": 0,
  "body_read_ms": 0, "body_complete": true, "api_key_auth_ms": 4, "user_queue_ms": 0,
  "account_select_ms": 16, "account_queue_ms": 0, "handler_body_read_ms": 0, "handler_total_ms": 2135,
  "upstream_attempts": 1, "connection_reused": false, "upstream_connect_ms": 1, "upstream_write_ms": 38,
  "upstream_first_byte_ms": 870, "upstream_response_headers_ms": 870, "upstream_stream_transfer_ms": 2134,
  "response_stream_complete": true,
  "gateway_pre_upstream_ms": 32, "upstream_first_output_ms": 1988, "retry_failover_ms": 0,
  "gateway_first_output_write_ms": 0, "gateway_first_output_total_ms": 2022
}
```

旧 6 段与新分段同时在同一个 JSON 中，历史记录缺字段时前端显示「未采集」。

## 3. 已知未采集项

- **安全审核**：当前 OpenAI 链路没有内容审核中间件，字段留空（前端显示「未采集」）。
- **大请求排队**：本仓库没有大请求队列，字段留空。
- WebSocket 透传、Anthropic/Gemini 专用分支未接入本采集器，仅旧 6 段可用。

## 4. 回滚

- 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-01a904fb3 /opt/sub2api/frontend/current`
- 后端：`docker stop <container>` → `install -m 0755 .../sub2api.bak-20260925-timing2 .../sub2api` → `docker start`
- 无数据库迁移；新增 JSONB 字段不会被旧版本读取。

## 5. 追加：安全审核分段、Anthropic/Gemini/WS 接入（06:45 JST）

| 提交 | 内容 |
| --- | --- |
| `f0c8b0e1b` | 新增 `security_audit_ms`（`runContentModeration` 唯一入口埋点）；`recordUsageCore` 合并采集器分段；`ForwardResult` 增加 `FirstOutputStages`；WS 两条路径与 chat completions 快照合并 |
| `059819093` | 修复：异步用量落库使用 `context.Background()`，采集器不在其中 → 改为在 handler 提交任务前用请求上下文同步合并（`/v1/messages` 等路径由此生效） |
| `295df3846` | Anthropic/Gemini 路径补结果标记 |
| `7eb5fce15` / `0fe66ec18` | 账号选择埋点：`SelectAccountForModelWithExclusions` 与网关 handler 的 5 处 `accountSelector.Select` |

**线上核验**：

- OpenAI `/v1/responses`（账号 126）：`outcome=success`、`account_select_ms=10`、`security_audit_ms=2`、`upstream_attempts=1`，其余分段齐全。
- Anthropic `/v1/messages`（账号 140，key 206）：`outcome=success`、`account_select_ms=6`、`security_audit_ms=2`、`user_queue_ms=1`、`connection_reused=true`、`upstream_attempts=3`、`upstream_connect_ms=818`、`upstream_first_byte_ms=2367`、`upstream_stream_transfer_ms=2371`、`response_stream_complete=true`。
- 容器 `Up (healthy)`、`RestartCount=0`；近 10 分钟 5xx = 0。

**仍未采集**：`account_queue_ms`（Anthropic 路径无账号槽等待）、`security_audit_ms` 之外的大请求排队（本仓库无该队列）。

**运行中的后端二进制**：`sha256=0fe66ec1869d4aadb5e8b68cc32c8fa303f2c972c0c3b4a986cefc844252b8fc`（由 `0fe66ec18` 构建）。中间态备份：`sub2api.bak-20260925-timing2`（`a49fb479…`）、`sub2api.bak-20260925-timing3`（`e8298eea…`）。
