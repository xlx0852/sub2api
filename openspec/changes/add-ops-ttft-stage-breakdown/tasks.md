## 1. Measurement Contract

- [x] 1.1 Add shared first-output stage timing types and monotonic reconciliation helpers.
- [x] 1.2 Define handler/turn admission timestamps that survive account retry and failover.
- [x] 1.3 Add unit tests for non-overlap, null handling, and total reconciliation.

## 2. Transport Instrumentation

- [x] 2.1 Instrument OpenAI HTTP/SSE fresh, reused, proxy, and canceled connection paths.
- [x] 2.2 Instrument OpenAI WebSocket egress connection acquisition and first-token paths.
- [x] 2.3 Instrument client WebSocket ingress per-turn timing.
- [x] 2.4 Accumulate failed-attempt, backoff, and account-switch time without resetting the gateway total.
- [x] 2.5 Add targeted Responses, Chat Completions, Messages compatibility, retry, and failover tests.

## 3. Persistence And Aggregation

- [x] 3.1 Add nullable `usage_logs.first_output_stages` JSONB persistence and domain/settlement plumbing.
- [x] 3.2 Extend hourly metrics with per-stage sum and sample-count fields.
- [x] 3.3 Implement raw, pre-aggregated, and mixed-window average/coverage queries.
- [x] 3.4 Add repository and migration regression tests.

## 4. Ops API And Dashboard

- [x] 4.1 Add the stage breakdown response contract to the Ops overview API.
- [x] 4.2 Add a responsive Ops Dashboard breakdown band with average, share, coverage, and precise tooltips.
- [x] 4.3 Add Chinese/English translations and frontend component tests.

## 5. Verification

- [x] 5.1 Run targeted backend tests, migration tests, race-sensitive relay tests, and backend build.
- [x] 5.2 Run frontend component tests, typecheck, and production build.
- [ ] 5.3 Verify raw versus hourly/mixed aggregation on representative seeded data.
- [x] 5.4 Deploy backend then frontend and compare stage sums, coverage, and gateway total against existing TTFT on live traffic.
