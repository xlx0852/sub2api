# affiliate-rebates Specification

## Purpose
将符合条件的管理员正向余额充值纳入现有推广返佣规则，同时保留余额流水与幂等边界。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Admin balance increases accrue affiliate rebates

The system SHALL treat a positive balance delta produced by an admin balance adjustment as a recharge for affiliate rebate calculation, using the existing affiliate policy settings and inviter relationship.

#### Scenario: Admin adds balance to an invitee

- **WHEN** an administrator increases a user balance and the user has an eligible inviter
- **THEN** the system credits the inviter's affiliate quota according to the configured rebate policy
- **AND** the user balance adjustment remains recorded as an admin balance history entry

#### Scenario: Admin reduces or leaves balance unchanged

- **WHEN** an administrator subtracts balance or sets a balance to a lower/equal value
- **THEN** the system SHALL NOT accrue an affiliate rebate
