# 六平台终态 usage 恢复能力盘点（2026-08-15）

## 结论

当前六个平台没有一个能覆盖 Sub2API 全部生产入口的“按 provider request ID 精确取回终态 usage”能力。

- **条件可恢复**：OpenAI 官方 API Key 的 stored Responses、xAI 官方 API Key 的 stored Responses。
- **不可按请求精确恢复**：Anthropic、Gemini `generateContent`、Antigravity、Kimi，以及 OpenAI OAuth/Codex、OpenAI Chat Completions、Grok OAuth/CLI/raw Chat 等当前生产形态。
- Anthropic/OpenAI 的组织 Usage API和项目已有 OpenAI/Kimi quota 查询只能提供时间桶或额度窗口，不能重建一条请求的 token、cache、reasoning、媒体字段。
- 为恢复而把当前 `store=false` 改成 `store=true` 会改变隐私、上游状态和续链语义，不属于本 change 的行为保持范围，禁止作为捷径。

因此 durable settlement 首期必须遵循：

1. 所有进入上游的主请求先持久化本地 `started` attempt；
2. terminal usage 到达后，以本地已解析快照作为唯一精确结算源并立即持久化 `ready` event；
3. 崩溃发生在 terminal usage 之前时，默认进入 `reconciliation_required`；
4. OpenAI/xAI stored Responses 查询只能作为条件式 adapter，不能成为通用恢复前提；
5. 聚合 Usage/Quota API只用于总量差异审计，不能自动分摊回单条用户账单。

## 能力矩阵

| 平台/生产形态 | 当前终态 usage 来源 | 当前稳定标识 | 官方/现有历史查询能力 | 精确恢复判定 | 首期处置 |
|---|---|---|---|---|---|
| Anthropic API Key/OAuth/Setup Token | JSON `usage`；SSE `message_start` + `message_delta` | `x-request-id`，消息体 `id` | Admin Usage API仅按分钟/小时/天，并按 API key/workspace/model 等聚合；无 request ID 维度和消息取回 API | 不支持 | terminal 前崩溃转待对账；Admin Usage 只做时间桶总量核对 |
| OpenAI 官方 API Key Responses，`store=true` 且非 ZDR | `response.completed.response.usage` 或非流式 response `usage` | response `id` + `x-request-id` | Responses API可按 response ID retrieve；官方说明 stored Responses 有应用状态保留期 | 条件支持 | 允许后续实现 retrieve adapter；必须核验取回对象含完整计费字段 |
| OpenAI OAuth/Codex、WSv2 | Responses terminal usage | response `id` + header request ID | 项目强制 `store=false`；现有 `/wham/usage` 仅返回额度窗口 | 不支持 | 不改 store；terminal 前崩溃转待对账 |
| OpenAI Chat Completions/其它非 stored 入口 | JSON/SSE chat usage | chat completion `id` + header request ID | 组织 Usage API为时间桶聚合，不支持 request ID | 不支持 | terminal 前崩溃转待对账 |
| Gemini AI Studio/Vertex `generateContent` | `GenerateContentResponse.usageMetadata` | body `responseId`；header `x-goog-request-id`/平台 request ID | `generateContent` 参考只定义 POST 生成；没有按 responseId 取回既有 GenerateContentResponse 的方法 | 不支持 | 开始保存 responseId，但只作审计；terminal 前崩溃转待对账 |
| Antigravity `v1internal` | 包装后的 Gemini `usageMetadata`，Claude 兼容路径再映射 | body `responseId` 可出现；当前结算主要保存 `x-request-id` | 内部 `v1internal:generateContent/streamGenerateContent`；仓库无历史 response/usage 查询实现，公开 Gemini API也不能替代内部账号查询 | 不支持 | attempt 保存 header/body 标识；terminal 前崩溃转待对账 |
| Grok/xAI 官方 API Key Responses，允许 store | Responses terminal usage | response `id`；`x-request-id`/`xai-request-id` | xAI 官方提供 `GET /v1/responses/{response_id}`，stored response 可取回 | 条件支持 | 仅官方 host + stored Responses 后续接 adapter；先验证 usage 完整性和保留窗口 |
| Grok OAuth CLI proxy、raw Chat、store-disabled 媒体 | Responses/Chat terminal usage或媒体结果 usage | response/chat/request ID，视入口而定 | CLI proxy/raw Chat 没有仓库内可用的历史精确查询；deferred Chat 是不同提交模式，不能恢复普通 Chat | 不支持 | terminal 前崩溃转待对账 |
| Kimi OAuth/API compatible Chat | Chat Completions JSON/SSE usage | chat completion `id` 和可能的 header request ID | 官方 Kimi Code 文档只公开 OpenAI/Anthropic compatible inference；项目 `KimiQuotaService.QueryUsage` 查询滚动额度窗口，不是请求级 usage | 不支持 | terminal 前崩溃转待对账；quota window 只用于总量审计 |

## 当前代码证据

### Anthropic

- `gateway_upstream_response.go` 在 SSE 事件中合并 usage，要求 terminal event 后才返回完整结果。
- `gateway_pipeline_claude.go` 把上游 `x-request-id` 放进 `ForwardResult.RequestID`。
- 仓库没有按 Anthropic message/request ID 读取历史响应的 client。

### OpenAI

- `openai_gateway_response_handling.go` 从 Responses terminal event 聚合 usage 和 response ID。
- `openai_codex_transform.go` 与 `openai_gateway_request_body.go` 对 OAuth/ChatGPT internal 路径强制 `store=false`。
- `OpenAIQuotaService.QueryUsage` 请求 ChatGPT usage window，只能维护限额窗口，不能恢复某次调用。

### Gemini

- `gemini_pipeline_messages.go`、`gemini_pipeline_native.go` 当前只把响应 header request ID写入 `ForwardResult.RequestID`。
- Gemini body 的 `responseId` 没有进入现有 settlement 输入；usage 来自 `usageMetadata`。
- `generateContent` 和 `streamGenerateContent` 路径没有历史 response reader。

### Antigravity

- `internal/pkg/antigravity/client.go` 只实现 `v1internal:generateContent` / `v1internal:streamGenerateContent` 等提交接口。
- `antigravity_pipeline_*` 使用 `x-request-id`，`antigravity_gateway_streaming.go` 从包装的 Gemini response 提取 usage。
- 没有按 `responseId` 或 `x-request-id` 读取历史结果的实现。

### Grok

- `openai_gateway_pipeline_grok.go` 同时保存 header request ID 和 Responses body ID。
- 官方 API Key 路径可走 `api.x.ai/v1/responses`；OAuth 主路径可走 `cli-chat-proxy.grok.com`，两者能力不能混为一谈。
- 图片优化配置可能把 `store` 设为 false，raw Chat 也不是 stored Responses。

### Kimi

- Kimi 走 OpenAI-compatible Chat Completions，终态 usage 由共享 OpenAI extractor 收集。
- `KimiQuotaService.QueryUsage` 明确读取 5h/7d 等滚动 quota window，不携带 request ID 或单请求 token。

## 官方资料

- [Anthropic Messages Usage Report](https://platform.claude.com/docs/en/api/admin/usage_report/retrieve_messages)：结果是时间桶聚合，支持 API key/workspace/model/service tier 等维度，没有 request ID 维度。
- [OpenAI data controls](https://platform.openai.com/docs/models/default-usage-policies-by-endpoint)：Responses 的应用状态取决于存储/ZDR策略；ZDR 会把 `store` 视为 false。
- [OpenAI Usage API](https://platform.openai.com/docs/api-reference/usage)：提供时间桶聚合，不提供单请求恢复。
- [Gemini GenerateContent API](https://ai.google.dev/api/generate-content)：响应包含 `usageMetadata` 和 `responseId`，但当前 GenerateContent 方法没有按该 ID 读取历史响应的接口。
- [xAI Responses retrieve](https://docs.x.ai/developers/rest-api-reference/inference/chat)：提供 `GET /v1/responses/{response_id}`，并由 `store` 控制是否保存输入和响应。
- [xAI deferred Chat Completions](https://docs.x.ai/developers/advanced-api-usage/deferred-chat-completions)：仅适用于显式 deferred 提交，不能用于恢复普通 Chat 请求。
- [Kimi Code API access](https://www.kimi.com/code/docs/en/)：公开 OpenAI/Anthropic compatible inference endpoint，未提供按 completion/request ID 读取历史结果的接口。

## 对测试床的直接要求

- fake upstream 必须分别模拟：完整 terminal、首段 usage 后断流、没有 terminal、stored response 可 retrieve、retrieve 404。
- OpenAI/Grok case 必须拆分 `stored Responses` 与 `store=false/raw Chat`，不能按平台名给出单一恢复结论。
- Gemini/Antigravity fixture 必须保留 body `responseId` 和 header request ID，验证 journal 同时保存二者。
- Anthropic/Kimi fixture 必须验证聚合 quota/usage 查询不能被误当作单请求恢复结果。
