## Context

`usage_logs.first_token_ms` currently measures from the start of one `Forward`/WebSocket turn attempt to the first visible upstream output. Handler admission, account wait, and earlier failed account attempts are not represented consistently. Existing Ops latency context fields overlap and are primarily error diagnostics, so they cannot be summed into a reliable successful-request breakdown.

## Goals / Non-Goals

- Goals:
  - Produce mutually exclusive gateway-side stages whose sum can be reconciled with gateway end-to-end first-output latency.
  - Preserve transport-specific facts while exposing one stable dashboard contract.
  - Keep long-window averages exact through sums and sample counts.
  - Show measurement coverage so nullable or unsupported stages are not mistaken for zero latency.
- Non-Goals:
  - Measure client-to-gateway upload time before the server receives bytes.
  - Measure Internet delivery or UI rendering after the gateway flushes the first output.
  - Claim that upstream network time can be separated from OpenAI queueing, prefill, or inference without upstream timing headers.
  - Change scheduling, retry, failover, timeout, or billing behavior.

## Decisions

### Stage contract

The successful streaming request SHALL expose these nullable durations:

- `gateway_pre_upstream_ms`: gateway admission, body handling, moderation, concurrency wait, scheduling, and request rewrite before the first upstream attempt starts.
- `upstream_connect_ms`: connection-pool acquisition plus TCP/TLS/proxy/WebSocket connection establishment across attempts. Reused connections contribute zero or only measurable acquisition time.
- `upstream_first_output_ms`: successful-attempt request write completion to the first visible upstream output. This is an upstream wait measurement, not a pure OpenAI inference measurement.
- `retry_failover_ms`: failed-attempt execution, retry backoff, and account-switch delay not already counted in connection acquisition.
- `gateway_first_output_write_ms`: first visible output recognition and transformation through downstream writer flush.
- `gateway_first_output_total_ms`: request/turn admission through downstream writer flush.

Instrumentation SHALL use monotonic timestamps and SHALL assign elapsed time to only one stage. The service SHALL also record a reconciliation delta internally during tests/diagnostics; the sum of available stages MUST remain within a small implementation-defined tolerance of the total.

### HTTP and WebSocket mapping

- HTTP/SSE uses `httptrace` or equivalent transport hooks to distinguish connection acquisition from request-written-to-first-output wait. Reused HTTP connections remain valid samples.
- WebSocket egress maps pool acquisition/dial to connection time and request-write-to-first-token to upstream first-output wait.
- Client WebSocket ingress starts the end-to-end timer when the gateway accepts the turn payload, not when the long-lived socket was originally established.
- Failover accumulation lives at the handler/attempt coordinator so a new `Forward` call does not reset the user-visible total.

### Persistence and aggregation

Stage values are stored in a nullable `usage_logs.first_output_stages` JSONB document. Historical rows remain null and are excluded from stage averages. Keeping one additive raw column also allows transport-specific nullable fields without expanding the hot insert statement for every stage.

For every stage, hourly aggregation stores `sum_ms` and `sample_count`. The dashboard computes `sum(sum_ms) / sum(sample_count)` across pre-aggregated full hours and raw head/tail fragments. It SHALL return coverage as `sample_count / eligible_streaming_requests` for the selected filters.

Average-only UI does not remove sample counts from the API because coverage is required to interpret averages safely.

### Dashboard presentation

The Ops Dashboard adds one unframed latency breakdown band near the existing TTFT summary. It shows a stable sequence of stages with average milliseconds, percentage of the measured gateway total, and a compact coverage indicator. Unsupported or missing stages display `--`, never `0 ms`.

## Risks / Trade-offs

- HTTP transport tracing can be subtle with proxies and reused connections. Mitigation: transport-specific tests for fresh, reused, proxied, retry, and canceled requests.
- Timestamp hooks on hot streaming paths add overhead. Mitigation: use monotonic `time.Time` values and scalar fields only; do not emit per-event persistence writes.
- Partial instrumentation could create misleading sums. Mitigation: nullable fields, coverage reporting, reconciliation tests, and no zero-filling.
- `upstream_first_output_ms` includes upstream network and provider processing. Mitigation: name and tooltip avoid the label "OpenAI inference time".

## Migration Plan

1. Add the nullable raw usage stage document and additive domain/settlement fields.
2. Instrument transports behind the existing forwarding paths with no scheduling or billing changes.
3. Extend raw and hourly aggregation with sums/counts and backfill no historical values.
4. Add the API response and dashboard band.
5. Validate locally, deploy backend first, then frontend; old frontend ignores additive response fields and new frontend handles absent fields.

Rollback removes the new UI and stops writing the additive stage document; existing nullable database fields may remain safely.
