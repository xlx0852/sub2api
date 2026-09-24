# Design: Guard Mirasim Account Model Mapping

## D1 为什么不改 `IsModelSupported` 的「空映射＝允许所有」语义

`backend/internal/domain/account_entity.go:880` 的 `IsModelSupported` 被整条调度链路复用：
空映射＝不限制，这是所有平台共享的历史语义。把它改成「mirasim 账号空映射＝拒绝所有」
会引出两个新问题：

1. **失败模式反转**：账号从「可能 422」变成「静默不接流量」，故障更难发现，
   而调度层并没有「本账号上游真实模型清单」这份权威数据。
2. **依赖动态上游**：判定要准确就必须在调度热路径上读上游 `GET /v1/models`
   （8s 超时 + 网络抖动），把一次纯内存判断变成可能阻塞/失败的 I/O。

生产事故的直接原因是**配置缺失**，不是语义错误。因此选择「暴露 + 显式修复」：
让管理端看得见、点得动，语义保持现状。

## D2 为什么默认 409，而不是自动覆盖

`model_mapping` 可能承载人工维护的别名（例如把用户请求的 `deepseek-v4.1-flash`
映射到上游真实 id `deepseek-flash`）。一次性覆盖会静默丢弃这类人工意图。
因此：

- 空映射 → 直接写入（无内容可丢）；
- 非空映射 → `409`，需请求体显式 `overwrite=true`。

## D3 为什么只生成 identity 映射

identity（`id -> id`）是可证明正确的下界：上游既然把这批 id 列为可用，
那么「原样转发」一定能被接受。别名映射需要人工知识（label 与真实 id 的对应关系），
机器猜错就是把 422 从「漏配」变成「配错」，更糟。

前端 `useModelWhitelist.buildModelMappingObject` 早已用同一约定
（白名单模式下 `mapping[m] = m`），此处保持一致。

## D4 为什么不注入 mirasim 元数据服务

`model_mapping` 与上游模型清单缓存无关：写入映射不改变清单内容，
读清单也不依赖映射。既有 `AccountTestService.FetchUpstreamSupportedModels`
已经封装了 mirasim 分支（含 `force=true` 与错误分类），直接复用即可，
不新增依赖注入、不触碰缓存失效逻辑。

## D5 路由与处理器落点

- 新处理器放独立文件 `backend/internal/handler/admin/account_mirasim_model_mapping.go`，
  避免继续膨胀 `account_handler.go`。
- 复用 `AccountHandler.accountTestService`（既有字段）与 `adminService`（既有字段），
  **不需要**新增 `NewAccountHandler` 参数或 `wire_gen.go` 改动。
- 路由注册在既有 `accounts.POST("/:id/models/sync-upstream", ...)` 之后：
  `accounts.POST("/:id/models/apply-mapping", ...)`。

## D6 凭据保全

`MergePreservingSensitiveCreds` 的语义是「非敏感键完全由 incoming 决定」，
因此**只传 `{"model_mapping": ...}` 会清空 `base_url` 等非敏感凭据**。
处理器必须先把账号现有凭据中的**非敏感**子键整体拷入 incoming
（`service.IsSensitiveCredentialKey` 过滤），再覆盖 `model_mapping`；
sensitive 子键交给 merge 逻辑保留。这样写入后的凭据集合与写入前完全一致，
只多/改了 `model_mapping`。
