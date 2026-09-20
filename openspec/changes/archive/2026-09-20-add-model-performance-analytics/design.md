## Context

Sub2API already stores usage token counts, request duration, first-token timing, request type, model, platform/account/group attribution, and error/traffic records. The admin Ops dashboard already supports time filters, platform/group filters, raw versus pre-aggregated queries, and Chart.js visualizations. The new capability should compose these existing facts instead of introducing a second telemetry pipeline.

## Goals / Non-Goals

- Goals:
  - Give administrators a fast view of model responsiveness and generation efficiency.
  - Make KPI definitions explicit and comparable across platforms where the source data exists.
  - Support request-level visual diagnosis for recent windows without unbounded API responses.
  - Show coverage for metrics that are unavailable on some transports or historical rows.
  - Keep the existing Ops dashboard and billing behavior backward compatible.
- Non-Goals:
  - Persist prompt/response content or reconstruct conversation context.
  - Claim that provider-side generation time is observable when only gateway timing is available.
  - Replace the existing health/error dashboard, usage billing pages, or scheduler metrics.
  - Add a new external observability dependency.

## Decisions

### Placement and scope

The feature is an additional `performance` section within `/admin/ops`. It uses the existing admin authorization and Ops feature flag. The default query covers all platforms with available metrics; a platform filter narrows the result. OpenAI/Codex requests receive generation-speed data when output tokens and a valid generation interval are available. Other providers can contribute request count, tokens, duration, and TTFT according to their recorded usage fields.

### Metric definitions

- Total requests: requests in the selected window represented by the existing usage/Ops query contract.
- Success rate: successful requests divided by total requests, with zero requests rendered as unavailable rather than 100%.
- TTFT: existing gateway first-token timing, represented by P50/P90 and sample coverage.
- Generation TPS: `output_tokens / ((duration_ms - first_token_ms) / 1000)` when output tokens are positive and the denominator is positive. The value is null otherwise; it is not backfilled from total duration.
- Response duration: existing request duration in milliseconds, represented by P50/P90.
- Input context size: `input_tokens` per request, represented by total and P50/P90 where available. Cache token fields remain separate and are not silently added to input tokens.
- Failure points: only numeric timing/status metadata is returned. Payloads and error bodies are never included in the performance response.

### Request points and query limits

The API returns at most a bounded number of request points, ordered by request time. Short windows use raw rows. Longer windows return a representative time-bucketed series and expose whether points are sampled/aggregated. KPI aggregates remain authoritative and use raw plus existing hourly aggregates according to the selected query mode.

### Comparison windows

The API accepts optional comparison windows. When omitted, the backend derives a same-length immediately preceding window. The frontend also supports named wave windows for the screenshot-style comparison, defaulting to local-time morning and afternoon ranges only when the administrator explicitly selects the wave comparison mode. Time zone is returned with the response metadata so labels are not ambiguous.

### Compatibility and performance

The response is additive and all new fields are nullable. Existing overview and token-stat endpoints remain unchanged. Repository queries must apply the existing platform/group/request filters and use bounded time ranges. Tests cover null metrics, zero denominators, platform filtering, comparison alignment, and request point limits.

## Risks / Trade-offs

- Generation TPS can be misleading for very short or provider-specific streams. Mitigation: require a positive generation interval, expose sample count/coverage, and label it as gateway-observed.
- Cross-provider semantics differ. Mitigation: retain nullable fields and show metric coverage instead of fabricating values.
- Large windows can be expensive. Mitigation: reuse hourly aggregation for KPI queries and cap request-level points.
- Local-time wave comparisons can confuse deployments across time zones. Mitigation: include the effective time zone and allow explicit comparison windows.

## Migration Plan

1. Add domain response types and a repository/service query for performance analytics.
2. Add the admin endpoint and contract tests without changing existing endpoints.
3. Add the Ops dashboard performance section, charts, filters, and translations.
4. Run backend tests, frontend unit tests, typecheck, and production build.
5. Validate aggregate results against existing overview/token statistics on representative data.

No database migration is required. Rollback consists of removing the new route and UI; existing data and endpoints remain intact.
