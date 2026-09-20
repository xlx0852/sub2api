# Change: Complete Grok Codex Translation

## Why

Sub2API 已经能把 Codex Responses 请求转发到 Grok，并清洗部分 xAI 不支持的字段和工具类型，但直连 Grok Responses 路径还没有形成双向翻译闭环。当前 `custom` 工具只在请求侧部分降级为 `function`，回程 JSON/SSE 不会按原始工具类型还原；`apply_patch` 甚至会被直接删除，后续 `custom_tool_call` 历史与输出也未转换成 xAI 可接受的 `function_call` 形状，导致 Codex 工具循环中断。

GrokGo 已验证的核心思路是：在请求进入 Grok 前记录原始 custom 工具集合并做协议降级，在响应返回 Codex 前按工具名做逆向还原。Sub2API 应把这一思路整合到现有 Grok 专用路径中，同时复用现有 SSE 帧解析、模型映射、计费、调度和提示缓存能力。

## What Changes

- 为 Grok Responses 请求建立 request-scoped 的 custom 工具翻译上下文
- 将 Codex `custom` 工具（包括 `apply_patch`）转换为 xAI `function` 工具，并保留自由文本输入语义
- 将输入历史中的 `custom_tool_call` / `custom_tool_call_output` 转换为 xAI `function_call` / `function_call_output`
- 将 custom `tool_choice` 同步转换为 function 选择，避免工具定义与选择类型不一致
- 在非流式响应中，把属于原始 custom 工具的 `function_call` 还原成 `custom_tool_call`
- 在流式响应中，完整还原 custom 工具的 item 类型、参数事件名和终态字段，并保持普通 function 工具不变
- 增加 Grok 直连路径的请求体、非流式响应和 SSE 回归测试

## Impact

- Affected specs: `grok-codex-translation`
- Affected code: `backend/internal/service/openai_gateway_grok.go`、Grok 响应处理链、Responses 工具兼容辅助代码与定向测试
- Preserved behavior: Grok 模型映射、提示缓存身份、账号调度、计费、媒体 REST 端点、非 Grok 平台路径

