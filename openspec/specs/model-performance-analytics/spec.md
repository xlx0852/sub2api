# model-performance-analytics Specification

## Purpose
向管理员提供有界的模型性能指标、请求趋势与时段对比，保留缺失值且不暴露请求内容。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Admins Can Inspect Model Performance KPIs

The system SHALL provide an admin-only model performance analysis view within the existing Ops dashboard. For a selected time window and supported filters, it SHALL report request count, success and failure counts, success rate, TTFT P50/P90, response duration P50/P90, generation TPS P50/P90 when derivable, and input context size statistics when available.

#### Scenario: Admin loads a supported performance window

- **WHEN** an authorized administrator selects a time range and opens performance analysis
- **THEN** the system returns KPI values scoped to the selected platform, model, group, and request type filters
- **AND** the response includes metric sample counts or coverage for nullable metrics
- **AND** zero-request metrics are represented as unavailable rather than fabricated zeros or a 100% success rate

#### Scenario: A metric cannot be derived

- **GIVEN** a request has no first-token timing, no output tokens, or a non-positive generation interval
- **WHEN** the system calculates performance metrics
- **THEN** the unsupported metric is excluded from its percentile/sample calculation
- **AND** the UI indicates partial coverage rather than displaying a misleading value

### Requirement: Performance Analysis Supports Bounded Request Trends

The system SHALL return a bounded request-level performance series for recent windows. Each point MAY contain request time, model, status, TTFT, generation TPS, response duration, input tokens, and a failure classification. The endpoint SHALL never return prompt content, response content, credentials, or raw upstream payloads.

#### Scenario: Admin switches the plotted metric

- **WHEN** the administrator selects TTFT, generation TPS, or response duration
- **THEN** the chart plots the corresponding numeric value against request time
- **AND** the selected metric's unit and missing-data behavior are reflected in the chart
- **AND** failure points remain visually distinguishable when status timing exists

#### Scenario: The selected window has more points than the response limit

- **WHEN** the number of eligible request records exceeds the configured point limit
- **THEN** the backend returns no more than that limit
- **AND** the response identifies whether points were sampled or bucketed
- **AND** KPI aggregates are not changed by the point limit

### Requirement: Performance Analysis Supports Period Comparison

The system SHALL support comparing the selected window with an immediately preceding same-length window or with explicitly supplied comparison windows. It SHALL return the effective time zone and window boundaries used for comparison.

#### Scenario: Admin compares two same-length periods

- **WHEN** the administrator enables comparison without supplying a second window
- **THEN** the backend derives the immediately preceding window of equal duration
- **AND** it returns KPI values and deltas for both windows using the same filters and metric definitions

#### Scenario: Admin compares explicit morning and afternoon waves

- **WHEN** the administrator supplies two valid wave windows and enables wave comparison
- **THEN** the system returns separate KPI summaries for both windows
- **AND** the labels include their effective local-time boundaries
- **AND** invalid or overlapping windows are rejected with a validation error

### Requirement: Performance Data Preserves Existing System Behavior

The new capability SHALL reuse existing usage and Ops measurements without changing request routing, scheduling, billing, error handling, or existing Ops API response contracts.

#### Scenario: Historical rows lack optional timing fields

- **WHEN** an administrator queries a window containing historical rows without TTFT or generation timing
- **THEN** those rows remain eligible for request totals and available metrics
- **AND** missing optional values are excluded from derived metric samples
- **AND** existing Ops overview and usage behavior remain unchanged
