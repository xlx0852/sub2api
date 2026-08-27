# Change: Add Ops TTFT Stage Breakdown

## Why

The current `first_token_ms` value is a single gateway-side latency measurement. It cannot show whether slow first output is caused by local admission and scheduling, connection acquisition, upstream first-output wait, retry/failover, or downstream flush. It also restarts for each account forwarding attempt, so it can understate the wait observed by a user when an earlier account fails.

## What Changes

- Record a gateway end-to-end first-output latency from request/turn admission to the first visible output flushed by the gateway.
- Record mutually exclusive first-output stages for local preparation, upstream connection acquisition, upstream first-output wait, retry/failover, and gateway output flush.
- Persist nullable stage values on successful streaming usage rows without changing billing or existing `first_token_ms` semantics.
- Aggregate exact sums, sample counts, and coverage for each stage in raw and hourly dashboard query modes.
- Add an Ops Dashboard stage breakdown that shows the selected window's average latency and sample coverage for every stage.
- Label the measurements as gateway-side and explicitly exclude client-to-gateway upload latency and gateway-to-client Internet/render latency.

## Impact

- Affected specs: `ops-observability`
- Affected code: OpenAI HTTP/SSE and WebSocket relay timing, usage settlement/domain persistence, Ent schema and PostgreSQL migrations, Ops raw/hourly aggregation, control-plane response types, Ops Dashboard UI and translations
- Compatibility: one additive nullable raw JSONB column, additive hourly columns, and additive API fields; existing clients and historical rows remain valid
- Billing/routing: no intended behavior change
