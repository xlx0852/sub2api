# Change: 接入 OpenAI 官方状态观测

## Why

当 Sub2API 出现 499/5xx 波动时，运维人员目前需要手工打开 OpenAI Status，再与本地错误时间线对照。系统应将 OpenAI 官方聚合状态作为一个独立、可降级的观测信号，并与 Sub2API 自身实测数据并列展示，避免将局部错误误判为全局事故，也避免将官方状态当作调度真相。

## What Changes

- 新增 OpenAI 官方状态采集器，固定访问 `https://status.openai.com/api/v2/summary.json`，解析整体状态、组件状态和活跃事件。
- 使用独立后台任务定时采集；多实例时使用分布式锁减少重复请求，远端请求不得进入网关请求热路径。
- 仅在官方状态内容发生变化时持久化状态快照，并通过作业心跳记录采集成功、失败和最后尝试时间。
- 新增管理员 Ops API，返回当前官方状态、重点组件、活跃事件、数据新鲜度和历史变更。
- 在 Ops Dashboard 展示 OpenAI 官方状态卡片，并在错误趋势上标记官方事件时间窗；明确区分“官方聚合状态”与“Sub2API 本地实测”。
- 官方状态源失败或超时时，继续提供最后一次成功快照并标记 `stale`；不改变账号可调度性、故障转移、限流、计费或客户端响应。
- 对官方状态从 degraded 转为 operational 的变化生成可观测事件，用于管理员通知与历史对照，但不自动改写已发生请求的根因归类。

## Impact

- Affected specs: `provider-status-observability`
- Affected backend: Ops service/repository/handler/routes, background job lifecycle and wiring, runtime settings, database migration
- Affected frontend: admin Ops API types, Ops Dashboard status card and error-trend correlation display, zh/en i18n
- External dependency: OpenAI Status public JSON endpoint; it is treated as a best-effort observability source rather than a stable gateway dependency
- Gateway behavior: no change to request forwarding, account scheduling, billing, rate limiting or failover
