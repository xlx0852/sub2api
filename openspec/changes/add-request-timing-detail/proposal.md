# Change: 请求耗时详情补齐分段采集（客户端 / 网关内部 / 上游 / 结果）

## Why

使用记录现在只能看到「首字 / 总耗时 / 首输出总耗时」加上 6 个粗分段（`gateway_pre_upstream_ms`、`upstream_connect_ms`、`upstream_first_output_ms`、`retry_failover_ms`、`gateway_first_output_write_ms`、`gateway_first_output_total_ms`）。排查一次 50 秒的请求时，无法回答「时间花在读请求体、并发排队、账号选择，还是上游等待」——运营侧对照其它网关的请求详情（含请求体读取、鉴权/路由、上游连接与响应头、尝试次数、连接复用等分段）后要求补齐同口径字段。

## What Changes

在**不新增数据库表**的前提下扩展详细采集：`usage_logs.first_output_stages` 已是 JSONB，直接扩充字段即可。

- 新增采集器：请求级时间线，支持命名的阶段与时间点，线程安全，可序列化为 JSON。
- 入站请求体：中间件包装 `c.Request.Body`，记录 首读等待 / 接收区间 / 读体阻塞累计 / 字节数 / 是否完整读完。
- 网关内部：API Key 鉴权、用户并发等待、账号选择、账号并发准入、Handler 读取请求体、Handler 总耗时。
- 上游（经 `httpUpstream.Do` 统一入口，用 `net/http/httptrace`）：连接获取（含复用标记）、写请求、等待上游首字节、响应头就绪、首 Token、响应流传输、出口尝试次数、响应流完整。
- 结果：成功/失败/取消/超时/传输错误/未完成，以及请求体完整接收标记。
- 前端「请求耗时详情」弹窗按 客户端 → Sub2API / Sub2API 内部 / Sub2API → 上游 / 请求结果 四组展示，缺失项显示「未采集」，并保留现有入站/上游路径、Token、缓存、映射行。
- 采样与成本：采集器为纯内存累加，仅在请求上下文存在时生效；未接入的协议路径（如 WebSocket 透传）保持原样，字段留空显示「未采集」。

**不改变**：现有 6 个字段的语义与旧记录展示（旧记录只显示已有分段）；不新增表、不新增迁移；不改变计费、调度与转发行为。

## Impact

- 受影响规范：`account-usage-presentation`（请求耗时详情新增分段要求）。
- 代码：`backend/internal/domain/first_output_stages.go`、`backend/internal/service/first_output_stage_tracker.go`（扩展/承接）、`backend/internal/service/openai_gateway_handler.go`、`backend/internal/repository/http_upstream.go`、新增 `backend/internal/pkg/requesttiming/`、新增入站体中间件、`frontend/src/components/admin/usage/UsageRequestTimingModal.vue` 与 zh/en i18n。
- 无数据库迁移、无环境变量变更；`first_output_stages` 为 JSONB，新增字段对旧客户端透明。
- 回滚：前端回退 release；后端回退二进制即可，新增字段不会被旧代码读取。
