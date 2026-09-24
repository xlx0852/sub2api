# Design: 请求耗时详情采集

## 存储

沿用 `usage_logs.first_output_stages`（JSONB）。不新增表、不新增迁移：明细随用量行一起保留/清理，旧记录缺少新字段时前端显示「未采集」。

## 采集器

`internal/pkg/requesttiming`：请求级时间线。

- `Collector`：`Start(ctx)` 返回带采集器的 context；`Mark(name)` / `Span(name) func()` / `Observe(name, time)` 记录时间点与区间；内部互斥锁累加，序列化输出毫秒整数与布尔标记。
- 只在前端关心的字段上做映射：`Snapshot()` 输出 `domain.FirstOutputStages`，空值用 `nil` 表示（前端显示「未采集」）。
- 采集器随 context 传递（`WithCollector` / `FromContext`），与现有 `FirstOutputStageTracker` 并存：旧字段继续由旧 tracker 产出，新字段由新采集器产出，最后合并进同一个 JSON。

## 接入点

| 分组 | 字段 | 采集方式 |
| --- | --- | --- |
| 客户端 → Sub2API | 首读等待 / 接收区间 / 读体阻塞累计 / 字节数 / 完整接收 | 中间件包装 `c.Request.Body`：首次 `Read` 记为接收起点，最后一次读到 `EOF` 记为接收终点；`Read` 阻塞时长累加；字节数累加；`EOF` 标记完整 |
| Sub2API 内部 | API Key 鉴权 | 鉴权中间件开始/结束标记 |
| | 用户并发等待、账号并发准入 | `acquireResponsesUserSlot` / `acquireResponsesAccountSlot` 前后标记 |
| | 账号选择（含槽位探测） | pipeline `StageSelect` 前后标记 |
| | Handler 读取请求体 | 中间件记录的读体阻塞累计时长 |
| | Handler 总耗时 | 中间件在响应写出结束时记录 |
| Sub2API → 上游 | 连接获取（含复用）、写请求、等待首字节、响应头就绪、流传输、尝试次数 | `httpUpstream.Do` 注入 `httptrace.ClientTrace`：`GotConn`（含 `Reused`）、`WroteRequest`、`GotFirstResponseByte`、`Do` 返回为响应头就绪；包装 `resp.Body` 统计到 `EOF` 的传输时长与完整性；每次 `Do` 计数为一次出口尝试 |
| 请求结果 | 成功/失败/取消/超时/传输错误/未完成、尝试次数、连接复用、请求体完整接收、响应流完整 | 由上述标记与转发结果推导 |

## 兼容与回滚

- 旧记录：只有 6 个旧字段，弹窗其余行显示「未采集」。
- 旧客户端：JSONB 新增字段不影响解析。
- 未接入路径（WebSocket 透传、Anthropic/Gemini 专用分支）：字段留空；后续按同一采集器补齐。
