## ADDED Requirements

### Requirement: Grok 额度感知软调度

系统 SHALL 在现有粘性会话和硬额度排除之后，使用新鲜 Grok billing 或 API quota 快照作为可选的账号调度软权重。

#### Scenario: 新鲜 billing 快照参与评分
- **WHEN** 多个可用 Grok 账号具有新鲜且成功的 billing 剩余额度数据
- **THEN** 系统在启用 quota headroom 权重时提高剩余额度更多账号的候选评分

#### Scenario: API quota 作为回退信号
- **WHEN** billing 快照不可用但 requests 或 tokens quota 窗口仍然新鲜有效
- **THEN** 系统使用最保守的有效剩余比例作为软权重

#### Scenario: 陈旧或损坏数据保持中性
- **WHEN** Grok quota 数据缺失、陈旧、过期或无法解析
- **THEN** 系统使用中性权重且不因此排除账号

#### Scenario: 粘性会话优先
- **WHEN** 会话已绑定到仍可用且未硬耗尽的 Grok 账号
- **THEN** quota 软权重不得破坏现有粘性账号选择语义

#### Scenario: 硬额度耗尽优先
- **WHEN** 现有 Grok quota 自动暂停逻辑确认账号已耗尽或 retry-after 仍生效
- **THEN** 系统在软评分前排除该账号
