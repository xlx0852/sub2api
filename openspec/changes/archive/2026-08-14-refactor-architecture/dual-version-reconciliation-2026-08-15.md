# Refactor Architecture Dual-Version Reconciliation (2026-08-15)

## Scope

This report compares the pre-refactor release source at `f94204d8a` with the
`refactor-architecture` candidate deployed to the Japan staging host. Production
at `code.sicts.shop` was not used for test traffic.

The two versions ran concurrently with separate PostgreSQL, Redis, and data
volumes. Both databases were restored from the same staging snapshot
(`SHA256 dc5aea177fa2a8eaed0b83c85074e1c4add37b54372d81cb66f7af69c740deea`).
Scheduled account tests were disabled in the clones, historical usage was
cleared, and the test user balance and API-key quotas were reset to identical
values before each controlled run.

## Automated harness

- `tools/refactor_dual_version_reconcile.py`
  - sends identical requests to old and new instances concurrently;
  - covers Anthropic Messages, OpenAI Responses, Grok Responses, Kimi Chat
    Completions, JSON and SSE;
  - optionally covers OpenAI Responses WebSocket;
  - compares HTTP status, content type, payload format, schema, normalized
    protocol/control fields, model, and canonical usage;
  - marks every request with `sub2api-dual-diff/<run>/<case>` so persisted usage
    can be queried without relying on generated request IDs.
- `tools/refactor_dual_version_fake_upstream.py`
  - provides deterministic JSON/SSE fixtures with fixed token and cache usage;
  - was served over a temporary private-CA HTTPS endpoint;
  - was only connected to the isolated comparison networks.

No API key or upstream credential is written to either script or report.

## Real-upstream comparison

The final real-upstream run covered eight HTTP/SSE cases plus OpenAI WebSocket:

- HTTP/SSE: both versions returned `200` for all eight cases.
- WebSocket: both versions completed the `101` handshake and emitted
  `response.completed` with identical usage and persisted WS fields.
- Response schema matched in all nine cases.
- Every response usage object was correctly split and persisted by its version.
- Model, requested/upstream model, endpoint, stream flag, rate multipliers,
  billing type, and reasoning dimensions matched in all persisted rows.
- Whenever upstream usage was identical, `total_cost` and `actual_cost` were
  exactly equal between versions.

The remaining real-upstream value differences were attributable to the upstream:
OpenAI cache hits and Grok generated/reasoning tokens differed between the two
simultaneous requests. Each version persisted and charged its own returned usage
correctly. In both databases:

`balance delta == SUM(usage.actual_cost) == SUM(api_keys.quota_used)`

Real-upstream report SHA256:

`8c7d13ef2db5119ee74221cc8d9664bfd63a6906d819a25a54cb3bbf7ec3bac6`

## Deterministic HTTPS replay

The deterministic run removed upstream randomness and covered the eight
HTTP/SSE cases:

- transport success: `8/8`;
- response schema equal: `8/8`;
- normalized protocol/control fields equal: `8/8`;
- canonical response usage equal: `8/8`;
- persisted usage rows per version: `8`;
- selected persisted usage JSON digest, old and new:
  `f86a5768f30c71571e19cf87b6f28a94`;
- old and new final balance: `9.999728`;
- old and new balance delta, usage actual-cost sum, and API-key quota-used sum:
  `0.000272`.

Deterministic report SHA256:

`9d8700b9dda3534000f1b8bfa6edb2434c2c453b5e0b59c65b8cef3d4037388a`

## Result and limitations

No response-contract, usage-mapping, or billing difference was found in the
covered Anthropic, OpenAI, Grok, Kimi, SSE, and OpenAI WebSocket paths.

This is evidence for the covered paths, not proof that the entire refactor is
complete or risk-free. Antigravity still lacks a real staging credential, Gemini
account creation is intentionally retired, and the separately classified Go test
failures remain outside this reconciliation run.

All temporary containers, cloned database/Redis/data volumes, private CA keys,
credential-bearing database snapshot, loopback listeners, and SSH tunnels were
removed after evidence collection. The normal staging deployment and US
production deployment remained healthy.
