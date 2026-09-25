## ADDED Requirements

### Requirement: Responses 工具摄取兼容 additional_tools 输入项

系统 SHALL 在把 Responses 请求转换为 Anthropic Messages 请求时，识别 `input[]` 中
`type == "additional_tools"` 的输入项，并把其 `tools[]` 合并进该请求的工具集，
同时 MUST NOT 把该输入项当作对话消息转换。合并顺序 SHALL 与既有 responses-lite 降级一致
（顶层 `tools` 在前、自 `additional_tools` 摄取的工具在后）。

当请求中同时存在顶层 `tools` 与 `additional_tools` 输入项时，系统 SHALL 合并两者；
当 `additional_tools` 载体存在但其 `tools` 为空时，系统 SHALL 剔除该载体且
MUST NOT 清空顶层 `tools`。

#### Scenario: Codex 0.155 形状的请求

- **WHEN** 请求顶层不含 `tools`，而 `input[0]` 为
  `{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"exec_command","parameters":{…}}]}`
- **THEN** 发往上游的 Anthropic 请求包含名为 `exec_command` 的工具，且其 `input_schema`
  等于请求中给出的 `parameters`
- **AND** 发往上游的 `messages` 中不包含该 `additional_tools` 项

#### Scenario: 两种来源并存

- **WHEN** 请求同时带顶层 `tools=[A]` 与 `input` 中的 `additional_tools` 携带 `[B]`
- **THEN** 上游工具集为 `[A, B]`（A 在前），且两者都不丢失

#### Scenario: 空载体

- **WHEN** `additional_tools` 项的 `tools` 为空数组，而顶层 `tools=[A]`
- **THEN** 上游工具集仍为 `[A]`，且 `messages` 中不含该空载体

#### Scenario: 不含该形状的请求不受影响

- **WHEN** 请求既无顶层 `tools` 也无 `additional_tools` 输入项
- **THEN** 转换结果与本次变更前逐字节一致

### Requirement: namespace 工具展平

系统 SHALL 递归展开 `type == "namespace"` 的工具（子工具键 `tools` 与 `children` 均须支持），
把命名空间内的子工具提升为该请求的平铺工具。系统 MUST NOT 改写子工具的名称，
且 MUST NOT 在展平过程中静默丢弃或覆盖同名工具。

#### Scenario: 单层 namespace 展平

- **WHEN** 工具集含
  `{"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec_command","parameters":{…}}]}`
- **THEN** 上游工具集包含且仅包含一个名为 `exec_command` 的工具
- **AND** 其名称未被加上任何前缀

#### Scenario: children 子键

- **WHEN** 命名空间改用 `children` 键承载子工具
- **THEN** 展开结果与使用 `tools` 键时一致

#### Scenario: 多层嵌套

- **WHEN** 命名空间内再嵌套命名空间
- **THEN** 系统递归展开直至得到叶子工具

#### Scenario: 同名冲突

- **WHEN** 展开后出现两个同名工具
- **THEN** 系统返回错误，且 MUST NOT 以静默覆盖的方式继续

### Requirement: 工具参数在流式与非流式之间保持一致

系统 SHALL 保证同一工具调用请求在流式与非流式响应中给出**完全一致**的 `arguments`
字符串，且该字符串 MUST 是合法 JSON。系统 MUST NOT 在参数前附加任何占位片段。

#### Scenario: 非流式参数为合法 JSON

- **WHEN** 客户端以 `stream=false` 调用 `/v1/responses`，且模型发起工具调用
- **THEN** 响应中 `function_call.arguments` 是合法 JSON（可被 `json.Unmarshal` 解析）
- **AND** 其内容等于模型实际产出的参数对象序列化结果

#### Scenario: 流式与非流式一致

- **WHEN** 分别以 `stream=true` 与 `stream=false` 发起同一工具调用请求
- **THEN** 流式 `response.function_call_arguments.done.arguments` 与非流式
  `function_call.arguments` 逐字节相同
