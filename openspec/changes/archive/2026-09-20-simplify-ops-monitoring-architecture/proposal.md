# Change: Simplify Ops Monitoring Architecture

## Why

The Ops monitoring subsystem currently carries redundant background processing and heavyweight logging storage:
1. `ops_preaggregation_daily` runs periodically and writes to `ops_metrics_daily`, yet `ops_metrics_daily` is completely unused across the entire application (no frontend, dashboard, or API query reads from it; all long-window dashboards use raw or hourly metrics). This creates unnecessary background database queries, lock contention, and storage consumption.
2. `ops_error_logs` stores unbounded upstream response bodies and error details, causing significant write amplification and table bloat during upstream outages.
3. Alert evaluation logic contains redundant complexity that can be streamlined into direct, clear health and error rate threshold evaluations.

## What Changes

- **Retire Daily Preaggregation Job**: Remove the unused `ops_preaggregation_daily` background job from `OpsAggregationService` and stop writes to `ops_metrics_daily`. Retain `ops_preaggregation_hourly` for dashboard aggregation.
- **Bound Error Log Payloads**: Truncate oversized error bodies and upstream error details to a sensible maximum length (e.g., 2KB) when persisting to `ops_error_logs` to prevent storage bloat while keeping all critical diagnostic info.
- **Streamline Alert Evaluation**: Simplify `OpsAlertEvaluatorService` by keeping straightforward error rate, status code, and latency threshold evaluations.
- **Ensure Full Compatibility**: Maintain 100% backward compatibility for all existing Ops Dashboard API endpoints, queries, and UI components.

## Impact

- Affected specs: `ops-observability`
- Affected code:
  - `backend/internal/service/ops_aggregation_service.go`
  - `backend/internal/repository/ops_repo_preagg.go`
  - `backend/internal/handler/ops_error_logger.go`
  - `backend/internal/service/ops_alert_evaluator_service.go`
- Compatibility: Zero breaking changes. Dashboard metrics and error query APIs continue to work seamlessly.
