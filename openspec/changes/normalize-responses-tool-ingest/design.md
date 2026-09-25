# Design: Normalize Responses Tool Ingest for Anthropic Upstreams

## Goals / Non-Goals

- Goals:
  - 让「客户端用 Responses、上游是 Anthropic 协议」这条路径**正确摄取工具**，
    使 Codex 0.155+ 能把工具真正交给上游模型。
  - 摄取规则**自洽在转换器内**，不依赖调用方是否先跑过 responses-lite 降级。
  - 非流式与流式的工具参数**逐字节一致**。
  - 不含这两种新形状的请求，行为逐字节不变。
- Non-Goals:
  - 不改 responses-lite 降级在 OpenAI 上游三路径上的既有行为（它已能跑，
    本次只是让 Anthropic 路径不再依赖它）。
  - 不为 Anthropic 协议发明命名空间语义（Anthropic 无此概念，采用展平）。
  - 不改平台/账号类型/quota/计费。
  - 不做工具结果的注入或改写（`function_call_output` ↔ `tool_result` 既有实现已覆盖）。

## Evidence（2026-09-25 生产实测，逐条可复现）

| 观测 | 值 |
| --- | --- |
| Codex 真实请求体 | 71810 B；顶层键 `model,input,tool_choice,parallel_tool_calls,reasoning,store,stream,include,prompt_cache_key,text,client_metadata` — **无 `tools`** |
| 工具的所在 | `input[0] = {"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec_command",…}]}]}` |
| 直连上游带工具（`/v1/messages`） | `stop_reason: "tool_use"`，`tool_use` 块 `name=get_weather, input={"city":"Tokyo"}` → **上游正常** |
| `/v1/responses` 顶层 `tools`（流式） | `response.function_call_arguments.done.arguments = '{"city": "Tokyo"}'` → **正确** |
| `/v1/responses` 顶层 `tools`（非流式） | `function_call.arguments = "{}{\"city\": \"Tokyo\"}"` → **非法 JSON** |
| `gateway_forward_as_responses.go` 对 responses-lite 的引用 | **0** |
| `convertResponsesToAnthropicTools` 的 `namespace` 分支 | **不存在**（落到 `default`，子工具全丢） |
| 三个模型经 Codex 的失败表现 | deepseek 输出 `\|\|DSML\|\|` 标记文本；kimi 输出 `\|open\|…\|sep\|` 标记文本；glm **编造执行结果**（无 `command_execution`） |

## Decisions

### Decision 1: 在转换器内摄取，而不是在路径上补一次 lite 降级

- Chosen: `ResponsesToAnthropicRequest` 自己认 `additional_tools` 输入项。
  Alternatives:
  - 在 `gateway_forward_as_responses.go` 里调 `applyOpenAIResponsesLiteForUpstream`：
    改动小，但把「能否拿到工具」变成**调用顺序**问题——多一条路径（未来新增构造器）
    就再漏一次，正是本次 bug 的成因模式。
  - 让 `demoteOpenAIResponsesLiteTools` 成为强制前置：会牵动 OpenAI 路径的既有语义
    （它同时处理 base_instructions / reasoning context / client_metadata），风险更大。
- Rationale: 转换器是**唯一必经点**，在此摄取即为不变量。

### Decision 2: namespace 采用「展平」，工具名保持原样

- Chosen: 递归展开 `namespace`（`tools` 与 `children` 两个子键都支持），子工具直接成为
  Anthropic 工具的平铺项，**工具名不加前缀**（`exec_command` 就叫 `exec_command`）。
  Alternatives:
  - 拼前缀（如 `functions.exec_command`）：回执里名字会变，客户端按 `name` 匹配工具，
    可能匹配不上；且 Anthropic 工具名对 `.` 的接受度未经上游确认。
  - 整体丢弃 namespace：等于回退到本次的 bug。
- Rationale: 客户端请求里 `namespace` 的语义是**分组**，不是名字的一部分；展平后
  `name` 与原请求内的工具名一一对应，回执天然可匹配。
- Risk: 若同一请求的两个命名空间下有**同名工具**，展平会冲突。当前 Codex 只发一个
  `functions` 命名空间，但需要**显式检测并去重/报错**，不能静默覆盖。

### Decision 3: 合并顺序与 lite 降级保持一致

顶层 `tools` 在前、从 `additional_tools` 取出的在后（与
`demoteOpenAIResponsesLiteTools` 的 `merged = append(existing, extracted...)` 相同），
避免同一条请求在两条路径上产生不同的工具顺序。

### Decision 4: 非流式聚合的 `{}` 前缀按「非法 JSON」处理

- 目标：非流式 `arguments` 与流式 `done.arguments` 完全一致。
- 定位方向：`buildResponsesOutputJSON` / `BufferedResponseAccumulator` 对 function_call
  参数的**初始化**（疑似先写入 `{}` 再追加 delta）。
- 约束：修完必须**双向**验证（同请求 stream=true/false 比对），因为该聚合函数也被
  `/v1/responses` 非流式与 compact 等路径复用。

## Risks / Trade-offs

- `apicompat` 是多桥共用包（Responses↔Anthropic、Responses↔Chat、Chat↔Responses），
  改 tool 摄取会影响所有调用方 → 必须跑全量 `./internal/pkg/apicompat/` 与
  `./internal/service/` 回归（后者含 6 个**既有**失败，需用 stash 法证明与本次无关）。
- 展平 namespace 后，若上游模型在 `tool_use.name` 里回了带前缀的名字，回执会匹配不上 →
  需在响应桥侧容忍（或做一次反查），本次先在测试里钉住「名字不改写」，实测若发现
  上游会加前缀再补反查。
- 本次不改 responses-lite，所以两条路径的工具**顺序**必须一致（Decision 3），
  否则同一模型在 OpenAI 上游与 Anthropic 上游上表现会不同。

## Migration

- 无数据库/配置/前端改动。回滚 = 还原 `apicompat` 两个函数 + 删测试。
- 部署后必须用**真实 Codex** 验证（单测无法覆盖「客户端真实形状」这一点，本次 bug 正是
  因为单测用的是手写顶层 `tools` 而漏掉了线上形状）。
