## Context

Sub2API 当前已经支持 Grok HTTP `/responses` 和 Grok HTTP bridge，但 Codex 官方客户端的主链路是 WebSocket。对 Grok Composer 类模型而言，官方客户端依赖连续会话、增量输入、`previous_response_id` 和会话级连接状态；HTTP bridge 每轮重建请求，虽然能兼容文本请求，但会削弱上游原生会话语义。

CLIProxyAPI 的稳定路径是：下游 WebSocket 请求进入 xAI 专用 executor 后，优先转发到 xAI WebSocket `/responses`，并维护 session、response ID 映射和连接复用。Sub2API 应融入同类能力，但必须保留现有 HTTP bridge，避免破坏图片、视频和非 WS 场景。

## Goals / Non-Goals

- Goals:
  - 让 Grok OAuth 账号在 Codex Responses WS 场景中可以使用原生 xAI/Grok WS 上游
  - 保留 `previous_response_id`，并维护下游 ID 到上游 ID 的显式映射
  - 按 execution session 复用 Grok 上游 WS 连接，连接失败时快速剔除
  - 保留 HTTP bridge fallback，并继续支持现有 Grok 图片、视频和 HTTP 请求
  - 将 Grok WS 连接指标纳入已有账号性能统计
- Non-Goals:
  - 不把所有 Grok 请求强制切到 WS
  - 不重写通用 OpenAI WSv2 状态机
  - 不移除现有 Grok HTTP bridge
  - 不改变非 Grok 平台的调度和传输模式

## Decisions

- Decision: Grok WS passthrough 使用显式开关，默认 `auto`
  - Why: 生产可灰度，失败可回退，不影响当前可用路径
  - Alternative considered: 全量替换 HTTP bridge
  - Rejected because: 图片、视频和部分 HTTP 客户端仍依赖 HTTP 路径

- Decision: Grok WS session 与 downstream execution session 绑定
  - Why: 避免把 turn、thread 和 transport connection 混成隐式状态，符合项目现有边界
  - Alternative considered: 按 account 全局复用连接
  - Rejected because: 容易串会话，并破坏 `previous_response_id` 语义

- Decision: 只在下游是 Codex/Responses WS 且账号支持 Grok OAuth token 时使用 Grok WS
  - Why: 这是 Composer 稳定性收益最大的最小集合
  - Alternative considered: 普通 OpenAI 兼容 stream 也走 Grok WS
  - Rejected because: 行为面更大，兼容风险高

- Decision: 保留 HTTP bridge fallback，但 fallback 必须记录明确原因
  - Why: 方便灰度期间定位是配置禁用、WS dial 失败、上游拒绝还是协议不支持
  - Alternative considered: WS 失败直接返回错误
  - Rejected because: 会降低现有生产可用性

## Technical Shape

1. 在 Grok 传输路由中新增 `grok_ws_passthrough` 模式解析，支持 `off | auto | force`
2. 新增 Grok WS URL builder，将 `https://cli-chat-proxy.grok.com/v1` 映射为 `wss://cli-chat-proxy.grok.com/v1/responses`
3. 新增 Grok WS headers builder，复用 OAuth token、Grok CLI headers、`x-grok-conv-id` 和透传 `x-grok-*` headers
4. 新增 Grok WS body builder：
   - 设置 `type=response.create`
   - 保留 `previous_response_id`
   - 保留 incremental input
   - 删除 HTTP/SSE 专属字段，如 `stream_options`
5. 新增 response ID mapper：
   - 下游 `previous_response_id` 入站时映射为上游 ID
   - 上游 response 事件返回时映射回下游 ID
6. 新增 session 级 Grok WS connection store：
   - 命中 session 时复用连接
   - read/write/ping 失败时立即剔除
   - `force` 模式失败返回错误，`auto` 模式失败回退 HTTP bridge
7. 将连接复用、dial、preflight、payload、events、fallback reason 写入已有 WS 指标链路

## Risks / Trade-offs

- 风险: Grok WS 上游协议和 HTTP `/responses` 字段存在差异
  - Mitigation: body builder 独立实现，不复用 HTTP bridge 的全部清洗逻辑
- 风险: response ID 映射错误会导致多轮上下文断裂
  - Mitigation: 单测覆盖 `previous_response_id`、多 turn、tool output 和重连 fallback
- 风险: 连接复用可能串会话
  - Mitigation: key 使用 account + downstream execution session，不按 account 全局复用

## Migration Plan

1. 新增 Grok WS passthrough 配置和只读解析
2. 实现 Grok WS URL/header/body builder
3. 接入 OpenAI WS forwarder 的 Grok 路由选择
4. 实现 response ID mapper 和 session connection store
5. 接入连接失败剔除和 HTTP bridge fallback
6. 补充指标与后台展示字段
7. 用测试 API key 验证 sync、stream、Codex WS
8. 生产蓝绿部署，仅对 Grok 分组启用 `auto`
