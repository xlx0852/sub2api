# Change: Normalize Responses Tool Ingest for Anthropic Upstreams

## Why

Codex 0.155（以及同类新客户端）**不再用顶层 `tools` 字段传工具**，而是把它塞在
`input[]` 的一个输入项里：

```json
{"type":"additional_tools","id":"at_…","role":"developer",
 "tools":[{"type":"namespace","name":"functions","tools":[
    {"type":"function","name":"exec_command","strict":false,"parameters":{…}}
 ]}]}
```

`gateway_forward_as_responses.go`（客户端要 Responses、上游是 Anthropic 协议的那条路）
**完全没有调用 responses-lite 降级**（该文件对 `responses-lite` 的引用数为 0），而
`additional_tools → 顶层 tools` 的搬运只实现在 `demoteOpenAIResponsesLiteTools`，且它
只在三条 **OpenAI 上游**路径上被调用（`openai_gateway_passthrough.go`、
`openai_gateway_pipeline_openai.go`、`openai_ws_ingress_session.go`）。

后果（2026-09-25 生产实测，deepseek-v4.1-flash / kimi-k3 / glm-5.3-flash 全部复现）：
`ResponsesToAnthropicRequest` 收到带 `additional_tools` 的请求 → 该项不是已知输入类型 →
被忽略 → **发往上游的工具数量为 0** → 模型只从系统提示里"知道"有工具、拿不到定义 →
各自编造工具调用文本（deepseek 输出 `||DSML||` 式标记、kimi 输出 `|open|…|sep|…`、glm 直接
编造执行结果）→ Codex 报 `Fatal error: tool exec invoked with incompatible payload`。

同时发现同一翻译层的第二个缺陷：**非流式聚合把工具参数拼坏**。
`/v1/responses` 非流式返回的 `function_call.arguments` 形如 `{}{"city":"Tokyo"}`
（多出 `{}` 前缀，非法 JSON），而同一请求流式返回的
`response.function_call_arguments.done.arguments` 是正确的 `{"city":"Tokyo"}`。

上游侧无问题，已实测：直连 `POST /v1/messages` 带 `tools` 时，上游正确返回
`stop_reason: "tool_use"` 与结构化 `tool_use` 块（`name=…, input={…}`）。

## What Changes

- 在 `apicompat` 的 Responses→Anthropic 请求转换中**原生支持工具摄取**：
  - 从 `input[]` 中识别 `type == "additional_tools"` 的输入项，取出其 `tools[]` 与顶层
    `tools` **合并**（顺序：顶层在前，注入项在后，与既有 lite 降级一致），并把该输入项
    从消息转换中剔除（它不是对话消息）。
  - `convertResponsesToAnthropicTools` **递归展开 `type == "namespace"`**（`tools` 与
    `children` 两种子键都支持），把命名空间内的子工具提升为 Anthropic 平铺工具；
    Anthropic 协议没有命名空间概念。
  - 命名空间内的**工具名保持原样**（如 `exec_command`），不拼接前缀——客户端按
    `name` 解析回执，改名会破坏回执匹配。
- 让 `gateway_forward_as_responses.go` 走的这条路径也能复用既有 lite 降级（二选一或叠加）：
  首选在转换器内自洽处理（见上），从而不依赖调用方是否先跑过 lite 降级。
- 修复非流式聚合的 `arguments` 拼接（`{}{…}` → `{…}`）：定位 `buildResponsesOutputJSON`
  与 `BufferedResponseAccumulator` 对 function_call 参数的初始化，去掉多余的 `{}` 前缀。
- 回归测试钉住：`additional_tools` + `namespace` 摄取、递归展平、名字不改写、
  非流式与流式 `arguments` 逐字节一致。

## Impact

- Affected specs: `openai-compat-bridge`（若不存在则新建能力名，见 tasks 1.1 的探测步骤）
- Affected code:
  - `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
    （`ResponsesToAnthropicRequest` 摄取 `additional_tools`；`convertResponsesToAnthropicTools`
    递归展平 namespace）
  - `backend/internal/pkg/apicompat/responses_to_chatcompletions.go` 或
    `openai_gateway_response_handling.go`（非流式聚合 `arguments` 的 `{}` 前缀）
  - 新增/扩展单测：`responses_to_anthropic_tools_test.go`（或同目录新文件）
- 兼容性：仅**新增**对两种既有线上形状（`additional_tools` 输入项、`namespace` 工具）的
  支持；不含这两者的请求路径逐字节不变。不改平台、不改计费、不改数据库。
- 风险面：`apicompat` 为多桥共用包，改动需全量回归
  `./internal/pkg/apicompat/`、`./internal/service/`（后者含 6 个既有失败，需先确认与本次无关）。

## Verification

- 单测：`go test ./internal/pkg/apicompat/ -run 'ResponsesToAnthropic' -count=1`
- 端到端（**必须在实测环境**）：
  1. 本地代理抓 Codex 真实请求，确认 `input[0].additional_tools` 结构（已抓，见
     `/tmp/codex-req.json`，71810 字节，顶层键无 `tools`）；
  2. 直连上游确认工具可用（已做：`/v1/messages` → `stop_reason: tool_use`）；
  3. 部署后跑 `codex exec -c model_provider=DeepSeek -c model=deepseek-v4.1-flash
     "Run the shell command: echo codex-tool-ok"`，期望出现真实
     `command_execution` 事件而非文本标记。
- 非流式对齐：同一请求分别以 `stream=true/false` 调用，断言 `arguments` 完全一致且为合法 JSON。
