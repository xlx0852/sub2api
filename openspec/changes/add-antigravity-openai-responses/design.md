## 实现

`GatewayHandler.Responses` 保留现有鉴权、并发、分组选号和结算；选择 Antigravity 时调用 `ForwardResponses`，其他账号继续 ForwardAsResponses。现有路由已经把 Antigravity 分组分发给该 handler，无须改变 OpenAI/Grok 的分发条件。

请求复用 apicompat 的 Responses 工具转换，但使用新导出的 ResponsesInputMessages 保留孤立工具结果供历史续接匹配，不使用 Chat Completions 的删除/排序规则。规范化消息直接转 Gemini contents；不经过 Anthropic 网络接口。

ForwardResponses 设置专用 relay 状态并执行既有 Gemini pipeline。relay 直接把 Gemini parts 转成 apicompat ChatDelta 数据结构，复用已有 Responses SSE 状态机生成 added/delta/done/terminal 事件。非流式请求消费同一状态机最终对象，避免维护两套工具响应规则。

## 续接和边界

原实现没有可直接复用的跨 provider Responses 历史存储，因此新增 service 实例持有的有界内存缓存。缓存原始 Gemini contents 和签名，以 API Key、分组、账号、模型隔离；30 分钟 TTL，128 条，单条 8 MiB，总序列化数据 64 MiB。终态下发前保存。store=false 不保存。查找失败返回错误，不 silently drop previous_response_id。

缓存不是持久化服务。客户端全量历史重放是默认可用路径。多实例/重启续接缺失明确返回 400；原账号不可选不会把签名转交给其他上游身份。

支持 function/custom/namespace(function) 工具；未实现的托管工具、图片、background、conversation、context_management、compact 明确拒绝。WebSocket 返回 426 并给出 HTTP/SSE 指引。

## 验证

模拟上游检查实际 v1internal 请求、指令、工具参数和工具结果；检查 JSON 与 SSE 输出、缓存/reasoning 用量、custom 工具回程、namespace 回程、输出长度终态、跨 Key 续接隔离、store=false、断流失败。执行 apicompat、路由、架构门禁和 Antigravity 回归，生成 Linux 非 embed 二进制。线上实测需要有剩余配额的账号，作为单独发布验证。
