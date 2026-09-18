# Change: Add Model Performance Analytics

## Why

The existing Ops dashboard exposes aggregate health, throughput, latency, TTFT, and token statistics, but it does not give administrators a single view for diagnosing model performance request by request. The requested view needs to correlate TTFT, generation speed, total response time, input context size, request time, and failures without exposing prompt or response content.

## What Changes

- Add a model performance analysis section inside the existing admin Ops dashboard.
- Add an additive backend API for KPI summaries, request-level performance points, and time-window comparisons.
- Support filters for time range, platform, model, group, and request type.
- Show total requests, success/failure counts, success rate, TTFT P50/P90, generation TPS P50/P90, response duration P50/P90, and input context size statistics.
- Add a metric switch for TTFT, generation TPS, and total response time, with successful requests plotted by request time and failures marked separately when timing data exists.
- Add configurable comparison windows for period-over-period analysis, with fixed default windows suitable for the dashboard.
- Use raw usage logs for short ranges and existing hourly aggregation or bounded sampling for longer ranges; do not load an unbounded request point set into the browser.
- Preserve existing billing, routing, Ops endpoints, and historical usage rows. Missing metrics remain null and are not treated as zero.

## Impact

- Affected specs: `model-performance-analytics`
- Affected code: Ops domain/API/repository aggregation, admin Ops dashboard components, frontend API types, translations, and focused tests.
- Database: no required schema change for the first version; existing usage and Ops timing/token fields are reused.
- Security: only aggregate and numeric performance data is returned; prompt bodies, response bodies, credentials, and raw upstream payloads are excluded.
