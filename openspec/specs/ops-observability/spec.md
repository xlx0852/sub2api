# ops-observability Specification

## Purpose
约束运维聚合任务、错误载荷大小与阈值告警计算，保持现有查询接口兼容并减少后台开销。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Ops Preaggregation Focuses On Hourly Metrics
The Ops aggregation subsystem SHALL execute hourly pre-aggregation to support long-window dashboard queries and SHALL NOT run redundant daily pre-aggregation background cron jobs.

#### Scenario: Background aggregation runs
- **WHEN** the OpsAggregationService runs its background scheduling loop
- **THEN** it executes hourly aggregation for completed hours
- **AND** it does not acquire daily leader locks or perform daily table backfills

### Requirement: Ops Error Logging Bounded Payload Size
The Ops error logging subsystem SHALL record diagnostic error details with bounded string lengths, preventing database table bloat from excessively large upstream or client payloads.

#### Scenario: Upstream returns large error response
- **GIVEN** an upstream provider returns an error with an exceptionally large response body or stack trace
- **WHEN** the error is recorded into `ops_error_logs`
- **THEN** the error body and upstream detail are truncated to the configured safe limit (e.g. 2KB) with a truncation indicator
- **AND** key status codes, message summaries, and request metadata remain preserved intact

### Requirement: Streamlined Threshold Alert Evaluation
The Ops alert evaluation engine SHALL evaluate system error rates and latency against configured thresholds using direct, deterministic calculation without redundant aggregation passes.

#### Scenario: Error rate exceeds threshold
- **GIVEN** the error rate in the evaluation window exceeds the configured threshold
- **WHEN** the alert evaluator runs its evaluation cycle
- **THEN** a firing alert event is emitted
- **AND** duplicate evaluations are avoided within the cooldown period
