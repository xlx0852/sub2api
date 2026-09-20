## Context

Sub2API maintains an Ops monitoring subsystem for error inspection, health metrics, and alerting. Over time, features like daily pre-aggregation were added without being integrated into dashboard views, leading to wasted database cycles and maintenance overhead.

## Goals / Non-Goals

- Goals:
  - Eliminate the `ops_preaggregation_daily` loop and unused daily writes to save DB I/O and locks.
  - Limit `ops_error_logs` body/detail sizes to prevent unbounded growth during incident storms.
  - Simplify alert evaluator code paths to be lean, reliable, and easy to maintain.
  - Preserve all existing Ops Dashboard metrics, TTFT stage breakdowns, and error investigation capabilities.
- Non-Goals:
  - Do not alter the hourly preaggregation (`ops_metrics_hourly`) which is required by the Ops Dashboard.
  - Do not change the schema or drop existing columns/tables in this phase to maintain database rollback safety.

## Decisions

- **Decision 1: Gracefully retire daily preaggregation job in Go service without schema migration**
  - Stopping the background loop in `OpsAggregationService` immediately eliminates the CPU and DB load. The legacy table remains intact in PostgreSQL for rollback safety.
- **Decision 2: Truncate error payloads at logging boundary**
  - In `OpsErrorLoggerMiddleware` and error recording helpers, cap text bodies at 2,048 bytes with a clear truncation suffix `... [truncated]`. Diagnostics remain actionable without storing multi-megabyte payloads.
- **Decision 3: Keep threshold-based alert evaluation transparent**
  - Preserve core P0/P1 alert rule evaluations (error rate, SLA rate, latency) with minimal execution overhead.

## Risks / Trade-offs

- *Risk*: A future feature might have wanted to read `ops_metrics_daily`.
  - *Mitigation*: The table remains available, and hourly metrics can be summed into daily figures trivially on demand.
