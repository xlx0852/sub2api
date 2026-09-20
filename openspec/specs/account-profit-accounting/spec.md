# account-profit-accounting Specification

## Purpose
界定订阅可用期与封禁生效时刻对账号配额窗口、利润投影的边界，避免把不可用时间计为可用额度。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Quota windows stop at subscription availability end

The system SHALL stop projecting quota-window occurrences for a subscription account at the controlling subscription cycle end, or earlier at the confirmed ban effective timestamp. Non-subscription accounts SHALL retain their existing rolling-window behavior.

#### Scenario: Subscription cycle expires

- **WHEN** a subscription account's current or latest controlling cycle reaches its end
- **THEN** the quota timeline SHALL show no weekly occurrence whose start is at or after that end

#### Scenario: Account is confirmed banned during a cycle

- **WHEN** a subscription cycle has an active termination with an effective timestamp before the cycle end
- **THEN** the quota timeline SHALL stop at the ban effective timestamp and SHALL NOT render a subsequent weekly window

#### Scenario: Metered account has a rolling snapshot

- **WHEN** a non-subscription account has a quota snapshot
- **THEN** the existing rolling-window projection SHALL remain unchanged
