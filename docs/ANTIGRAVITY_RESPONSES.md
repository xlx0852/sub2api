# Codex 使用 Antigravity Gemini

为本站 API Key 绑定 Antigravity 分组，选择该分组可用的 Gemini 文本模型。客户端使用 OpenAI Responses 协议：

```toml
model_provider = "sub2api-antigravity"
model = "gemini-3.8-flash"

[model_providers.sub2api-antigravity]
name = "Sub2API Antigravity"
base_url = "https://your-sub2api.example/v1"
env_key = "SUB2API_API_KEY"
wire_api = "responses"
```

模型名须与本站分组模型目录和账号映射一致；密钥使用本站 API Key。

支持 `POST /v1/responses` 和 `POST /responses` 的文本 JSON/SSE、instructions、reasoning effort、输出上限、结构化 JSON、function/custom 工具（包括 apply_patch）、additional_tools 以及 function namespace。函数及 custom 工具的调用 ID 和结果在后续请求中必须匹配。namespace 子项目前只支持 function。

Gemini 的原始请求入口继续可用。Antigravity 不支持图片输入/输出、托管工具（如 web_search/tool_search）、后台任务、conversation、context_management、远端 compact 或存储响应 CRUD；这些能力返回明确错误。WebSocket 探测返回 426 `websocket_not_supported`，客户端应使用 HTTP/SSE。

## 多轮请求

- 推荐 Codex 以 `store=false` 重放完整 input 历史；转换器保留 function/custom 调用与结果，不静默丢弃不匹配的工具结果。
- `store=true`（或省略 store）时，完成的响应可通过 `previous_response_id` 续接。网关缓存原始 Gemini contents（包括 thoughtSignature），绑定 API Key、分组、账号和实际模型；续接时优先选择原账号。
- 当前续接缓存为单进程内存：30 分钟 TTL、最多 128 条、单条序列化数据上限 8 MiB、总序列化数据上限 64 MiB。不承诺跨重启、滚动部署或多个 API 实例共享续接 ID；超过容量会淘汰旧项。
- 缓存缺失、过期或所有者/模型不一致时返回 400。客户端需要去掉 previous_response_id 并重发完整历史；网关不会忽略 ID 后只发送增量输入。
- 不接受其他 provider 的 opaque encrypted reasoning。可使用当前 provider 输出的可见 reasoning summary 作为文本历史。

上游未发送 finishReason 就断流时返回错误；已经开始 SSE 的请求以 response.failed 终止，不伪造 response.completed。达到输出上限使用 response.incomplete；usage 中保留缓存读取和 reasoning token 数据，计费继续走既有网关结算入口。

本地测试使用模拟上游覆盖双向请求内容和事件语义。生产接入仍需有可用 Gemini 配额的账号验证真实 Codex 工具回合；普通 health 或未授权 401 不能代替这一验证。
