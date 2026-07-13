## Context

Sub2API 当前存在两套相关但边界不同的能力：

1. `backend/internal/service/openai_gateway_grok.go` 负责 Codex/OpenAI Responses 到 Grok Responses 的直连清洗；
2. `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` 已经为 Responses↔Chat 桥实现了较完整的 `custom`↔`function` 双向转换。

缺口集中在第一条直连路径。`sanitizeGrokResponsesTools` 会把部分 `custom` 工具改成 `function`，但没有保存原始 custom 工具名，也没有处理 custom 调用历史和响应回写；同时对 `apply_patch` 做了特殊丢弃。因此请求可以到达 Grok，不代表 Codex 的完整工具循环可以继续。

## Goals / Non-Goals

- Goals:
  - 让 Codex custom/freeform 工具通过 Grok Responses 完成多轮调用闭环
  - 同时覆盖 JSON 与 SSE 返回路径
  - 复用现有稳定的 SSE 帧处理和 Responses 类型，不引入独立代理进程
  - 保持普通 function、xAI built-in 工具和非 Grok 平台行为不变
- Non-Goals:
  - 不复制 GrokGo 的桌面 OAuth、MCP 注入、AGENTS 注入或本地产物目录
  - 不自动给所有请求注入 `x_search`、`web_search` 或 `image_generation`
  - 本变更不实现 GrokGo 的服务端 image generation tool loop；Sub2API 现有图片 REST/计费链路继续独立工作
  - 不改变 Codex 高保真 OpenAI 上游路径的 `previous_response_id`、turn state 或 WS 生命周期策略

## Decisions

- Decision: 在 Grok 请求入口生成 request-scoped 翻译上下文
  - 上下文至少记录原始 custom 工具名集合
  - 请求重试、账号 failover、非流式回程和流式回程共享同一上下文
  - 不把该集合写入数据库、Redis 或跨请求缓存

- Decision: custom/freeform 工具统一降级为单字段 function schema
  - `custom` 工具转换为 `function`
  - 自由文本通过 `{"input":"..."}` 传递
  - `apply_patch` 与其他 custom 工具采用相同规则，不再特殊删除
  - 若 custom 工具原本含可用 schema，则仍以自由文本 contract 为准，避免把 grammar 误当 JSON schema

- Decision: 输入历史与工具定义成对转换
  - `custom_tool_call.input` 转成 JSON 字符串形式的 function `arguments`
  - `custom_tool_call_output` 转成 `function_call_output`
  - `call_id`、`name` 和顺序保持不变
  - custom `tool_choice` 转成等价 function 选择

- Decision: 按原始工具名做响应逆向还原
  - 只有名字属于本次请求 custom 集合的 `function_call` 才还原
  - 普通 function 工具绝不改写
  - 非流式回程生成 `custom_tool_call`，使用自由文本 `input`
  - 流式回程同步转换 `response.function_call_arguments.*` 为 `response.custom_tool_call_input.*`，并确保 added/done/completed 中 item 类型一致

- Decision: 在现有 SSE 帧级管线中改写，而不是按网络 chunk 做字符串替换
  - 网络 chunk 可能切断一条 JSON event；逐 chunk 改写会漏事件或破坏 JSON
  - 现有 handler 已按 SSE 行/帧处理，应在完整 `data:` payload 上做结构化改写
  - 未发生 custom 工具改写时尽量保持原始 payload 不变

## Risks / Trade-offs

- 风险: Grok 可能为 custom 工具返回非对象 arguments
  - Mitigation: 兼容 `{"input": string}`、单字符串字段和原始字符串三种提取方式，并用测试锁定
- 风险: SSE added/delta/done 类型不一致导致 Codex 丢弃工具调用
  - Mitigation: 用完整事件序列测试 item 类型、事件类型、call_id、name、input 和终态 output
- 风险: 改写误伤普通 function 工具
  - Mitigation: 所有逆向转换必须命中 request-scoped custom 工具名集合
- 风险: 与现有通用 Responses↔Chat 桥重复逻辑漂移
  - Mitigation: 优先抽取无平台状态的 custom 工具编码/解码 helper；不把 Grok 路由和通用桥强行合并

## Migration Plan

1. 先补 Grok 直连 custom 工具请求、历史输入、JSON 回程和 SSE 回程失败用例
2. 引入 request-scoped 翻译上下文与纯函数 helper
3. 接入 Grok 请求 patch 与现有响应处理链
4. 跑 Grok unit tests、apicompat tests 和相关 service 回归测试
5. 通过现有蓝绿流程部署；异常时回滚应用版本，无数据迁移

## Open Questions

- 后续是否单独提案，把显式 `image_generation` 工具桥接到现有 Grok 图片 REST/计费链路，而不是像当前一样从 Grok 文本请求中丢弃

