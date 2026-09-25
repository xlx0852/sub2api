# Tasks

> 状态：**实现中**。已完成：工具摄取 + namespace 展平（tasks 2.1–2.6、4.1、4.2、4.3.1）。
> 未完成：freeform/custom 工具双向映射（本轮实测新增发现，见 2.7–2.9）、
> 非流式 `{}` 前缀（task 3）、部署与真实 E2E（task 5）。

## 0. 实现期新发现（必须做，否则任务目标达不成）

抓到的真实 Codex 请求体（`testdata/codex_additional_tools_request.json`）显示，
`functions` 命名空间里的三个子工具形状**不同**：

| 工具 | type | parameters | format |
| --- | --- | --- | --- |
| `exec` | **custom** | ❌ 无 | ✅ lark 语法 |
| `wait` | function | ✅ | — |
| `request_user_input` | function | ✅ | — |

`exec` 是 **freeform（custom）工具**：只有 id 名与一段 lark 语法格式，没有 JSON schema。
若只把它当普通 function 转发，`normalizeAnthropicInputSchema("")` 会给出
`{"type":"object","properties":{}}`（零参数工具）——模型无法表达“执行这条命令”。
两向都要处理：

- **请求向**：`custom` + `format` → Anthropic 侧合成单一字符串参数
  （`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`），
  并把 `format.definition` 拼进 description（让模型知道语法）。
- **响应向**：模型对这类工具的 `tool_use` 必须还原成 **`custom_tool_call`**（带
  `response.custom_tool_call_input.delta/done`），而不是 `function_call`；
  否则 Codex 按声明类型校验，报 `incompatible payload`。

**可直接照抄的参考实现**：`chatcompletions_responses_bridge.go` 已完整实现 custom_tool_call
生命周期（`CustomToolNames` 集合 + `response.custom_tool_call_input.delta/done` + 
`itemType = "custom_tool_call"`，注释：“codex 只按该类型路由”）。
Anthropic 桥只需把工具的 custom 名单从请求 plumb 进 `AnthropicEventToResponsesState`，
并复用同一套事件形状。

## 0.5 架构修正（实现期发现，取代原 2.7–2.9 的写法）

原计划在 `apicompat` 里自己实现 custom/freeform 降级与回程还原 —— **不必要**。
仓库已有整套子系统，只是入口门控排除了本路径：

| 现有实现 | 位置 |
| --- | --- |
| `apicompat.FlattenResponsesNamespacesExcept(body, except)` | namespace 展平 + 名字映射 |
| `apicompat.DowngradeResponsesCustomTools(body)` → `customNames` | custom/freeform 降级为 function + 单一 `input` 字符串 schema（注释明说 `exec` 即此类） |
| `restoreOpenAIResponsesNamespacePayload(c, payload)` | 回程还原 namespace + `custom_tool_call`（在 9 处出/入站点已接好，含 CC 回退） |
| `shouldFlattenOpenAIResponsesNamespaces(account, …)` | **门控**：仅 `AccountTypeOAuth` 或 `IsExternalBridge()` 为真 |

因此改为（tasks 2.7–2.9 作废，替换为下面的 2.10–2.13）：

- [ ] 2.10 入站：让「客户端 Responses → Anthropic 上游」路径执行
      `FlattenResponsesNamespacesExcept` + `DowngradeResponsesCustomTools`，
      并把 `namespaceNames` / `customNames` 写入 gin ctx（复用既有 setter）
- [ ] 2.11 出站：`handleResponsesStreamingResponse` 对每个待发事件 payload 调
      `restoreOpenAIResponsesNamespacePayload`；非流式响应体同样处理
- [ ] 2.12 门控：优先在**调用点**判断（不动 `shouldFlattenOpenAIResponsesNamespaces` 的既有语义，
      避免影响 OAuth/external bridge 路径）；若确需扩门控，必须同时覆盖
      transport/WS 分支并加回归
- [ ] 2.13 保留已实现的 apicompat 摄取/展平（2.1–2.6）作为**最后一道防线**：
      调用方漏做规范化时工具不会再次凭空消失（正是本次故障模式），并已有测试
- [ ] 2.14 单测：custom 工具降级后 schema 为单一 `input` 字符串 + description 含语法；
      回程把 `tool_use` 还原为 `custom_tool_call` 且 namespace 名还原

## 1. 前置确认

- [x] 1.1 `openspec list --specs | grep -iE 'responses|compat'` 确认无同名能力；
      若有更贴切的能力名，改本变更的 spec delta 目录名（当前用 `responses-tool-ingest`）
- [x] 1.2 复核三条证据仍成立（可在本地一次性复现）：
      ① Codex 请求顶层无 `tools`、工具有在 `input[].additional_tools`；
      ② 直连上游 `/v1/messages` 带 `tools` → `stop_reason: tool_use`；
      ③ `grep -c responseslite internal/service/gateway_forward_as_responses.go` = 0
- [x] 1.3 记录基线：`go test ./internal/service/ -count=1` 的 6 个既有失败清单
      （antigravity 限流 ×3 + Codex 图像桥 ×3），实现后用同一清单比对

## 2. 请求方向：工具摄取（核心）

- [x] 2.1 `ResponsesToAnthropicRequest`：扫描 `req.Input`，取出 `type == "additional_tools"`
      的项，合并其 `tools` 到顶层工具集（**顶层在前、摄取项在后**），并把该项从
      消息转换中剔除（它不是对话消息，不得进入 Anthropic messages）
- [x] 2.2 该摄取逻辑抽成独立函数（如 `extractAdditionalTools(input []ResponsesInputItem) (tools []ResponsesTool, filtered []ResponsesInputItem, modified bool)`），
      便于单测与复用
- [x] 2.3 `convertResponsesToAnthropicTools`：新增 `case "namespace"`，**递归**展开
      `Tools` 与 `Children` 两个子键；子工具走既有 `function`/`custom` 分支
- [x] 2.4 展平冲突检测：同一父级下出现**同名**工具时报错（而不是静默覆盖）
- [x] 2.5 工具名**不加前缀**；在函数注释里写明「客户端按 name 匹配回执，改名会破坏配对」
- [x] 2.6 空 `additional_tools` 载体（`tools` 为空数组）：剔除载体但不产生工具，
      不得因此清空顶层 `tools`
- [~] 2.7（作废）改由 2.10–2.14 复用既有子系统实现
- [ ] 2.8 把请求侧的工具 custom 名单 plumb 进响应转换状态
      （参照 `ChatCompletionsToResponsesStreamState.CustomToolNames`）
- [ ] 2.9 响应向：custom 工具的 `tool_use` → `custom_tool_call` +
      `response.custom_tool_call_input.delta/done`（不再发 `function_call`）

## 3. 响应方向：非流式聚合的参数完整性

- [ ] 3.1 定位 `function_call.arguments` 出现 `{}` 前缀的写入点
      （候选：`buildResponsesOutputJSON` / `BufferedResponseAccumulator.ProcessEvent` /
      `collectRawResponsesOutputItemsFromSSE`）
- [ ] 3.2 修正后：非流式与流式的 `arguments` 必须**逐字节一致**且为合法 JSON
- [ ] 3.3 检查同一聚合函数被复用的其它路径（非流式 `/v1/responses`、compact）不受影响

## 4. 测试

- [x] 4.1 新增 `responses_to_anthropic_additional_tools_test.go`
  - [x] 4.1.1 `input[].additional_tools` → 顶层 tools 被摄取，`input` 中不再保留该项
  - [x] 4.1.2 顶层 `tools` 与 `additional_tools` **同时存在** → 合并、顺序正确、无重复
  - [x] 4.1.3 空载体 → 剔除但不清空顶层 tools
  - [x] 4.1.4 非 `additional_tools` 的输入项逐字节不变
- [x] 4.2 namespace 展平测试
  - [x] 4.2.1 `type=namespace` + `tools[]` → 子工具平铺且 `input_schema` 正确
  - [x] 4.2.2 `type=namespace` + `children[]` → 同上（两种子键都覆盖）
  - [x] 4.2.3 多层嵌套 namespace → 递归展开
  - [x] 4.2.4 同名冲突 → 返回错误
  - [x] 4.2.5 工具名不被改写（断言 `name == "exec_command"`）
- [x] 4.3 端到端形状回归（**新增**，防本次这类"单测形状与线上不符"）
  - [x] 4.3.1 用**抓取的真实 Codex 请求体**（脱敏后放入 testdata）作为输入，断言上游
        Anthropic 请求里工具数 ≥ 1 且含 `exec_command`
  - [ ] 4.3.2 非流式 vs 流式 arguments 一致性（同请求两种 stream）
- [ ] 4.4 回归：`go test ./internal/pkg/apicompat/ -count=1` 全绿

## 5. 验证与部署

- [x] 5.1 本地：`go build ./...`、`gofmt -l`、`go vet` 干净
- [ ] 5.2 全量回归：`./internal/pkg/apicompat/` + `./internal/handler/...` + `./internal/server/...` 全绿；
      `./internal/service/` 仅剩 1.3 记录的 6 个既有失败（用 stash 法证明）
- [ ] 5.3 部署：交叉编译 linux/amd64（`-tags embed`，复用现有 `web/dist`，无前端改动）
      → 备份 `sub2api.bak-*` → `docker stop/cp/start` → 落盘探针（新增标识符 grep）
- [ ] 5.4 **真实 Codex 端到端**（必须）：
      `codex exec -c model_provider=DeepSeek -c model=deepseek-v4.1-flash "Run the shell command: echo codex-tool-ok"`
      期望：出现 `command_execution` 事件、`exit_code=0`，**不再**出现文本标记 / 编造结果
- [ ] 5.5 同法验证 `kimi-k3`、`glm-5.3-flash`
- [ ] 5.6 回归 GPT 路径：`codex exec "Reply with exactly: ok"` 正常（确认未破坏原生路径）
- [ ] 5.7 把三个模型在 `~/.codex/model_catalog.json` 里恢复为可见（当前被隐藏，因为
      工具不可用），仅在本变更部署并验证通过后执行

## 6. 归档

- [ ] 6.1 `openspec archive normalize-responses-tool-ingest --skip-specs --yes`
- [ ] 6.2 把排查方法沉淀进 `docs/`（用本地代理抓 Codex 真实请求 + 直连上游二分，
      避免下次再从"模型不听话"这个错误方向查起）
