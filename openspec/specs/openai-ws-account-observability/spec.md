# openai-ws-account-observability Specification

## Purpose
记录 OpenAI WS 连接预检、获取和队列指标，并提供有时限的连接健康反馈与账号性能展示。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: WS Preflight Failures Lower Account Priority Temporarily

The OpenAI account scheduler SHALL temporarily lower the priority of accounts with recent WS preflight failures so that normal non-sticky selection avoids accounts that are actively reconnecting.

#### Scenario: Account with recent preflight failures is deprioritized

- **GIVEN** two eligible OpenAI accounts have the same configured priority
- **AND** one account has recent WS preflight ping failures
- **WHEN** the scheduler compares the two accounts for a non-sticky request
- **THEN** the account with recent WS preflight failures is treated as lower priority
- **AND** the penalty expires after the recent-failure window passes

### Requirement: Account Performance Includes WS Connection Metrics

The system SHALL persist and aggregate WS connection metrics for account performance diagnostics, including connection reuse, preflight failures, connection acquisition time, upstream payload size, event count, and queue wait time.

#### Scenario: Admin views account performance diagnostics

- **GIVEN** a WSv2 request records connection metrics
- **WHEN** an admin views account performance in account management
- **THEN** the performance summary includes connection timing and preflight failure hints
- **AND** the detailed tooltip can distinguish connection/queue/payload factors from total generation time
