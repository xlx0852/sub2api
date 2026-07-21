## ADDED Requirements

### Requirement: Grok 请求体保守优化

系统 SHALL 仅对发送至 Grok 上游的 Responses 与 Chat 请求执行可配置的保守请求体优化，并保持当前输入、工具调用标识和非 Grok 请求语义不变。

#### Scenario: 精确重复历史图片去重
- **WHEN** Grok 多轮请求包含多个字节完全相同的 `data:image/...` 历史图片
- **THEN** 系统仅保留最新副本，并保留远程 URL、非完全相同图片和当前输入中的图片

#### Scenario: 图片请求关闭上游存储
- **WHEN** Grok 请求包含图片内容
- **THEN** 系统在上游请求中设置 `store=false`

#### Scenario: 超过软预算时截断历史工具输出
- **WHEN** Grok 请求超过配置的软体积预算并包含大型历史工具输出
- **THEN** 系统以 UTF-8 安全方式保留头尾和截断标记，同时保留最新工具输出与 call ID

#### Scenario: 无法满足硬预算
- **WHEN** 安全优化后 Grok 请求仍超过配置的硬体积预算
- **THEN** 系统返回 HTTP 413 `invalid_request_error`，且不发送截断不完整的上游请求

#### Scenario: 非 Grok 请求隔离
- **WHEN** 相同负载发送至非 Grok 平台
- **THEN** 系统不应用 Grok 请求体优化
