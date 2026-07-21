## 1. Protocol Contract

- [x] 1.1 Add a pre-handshake Grok group guard to the Responses WebSocket handler.
- [x] 1.2 Return a deterministic unsupported-WebSocket error without sending `101 Switching Protocols`.
- [x] 1.3 Keep Grok HTTP Responses/SSE routes unchanged for both `/responses` and `/v1/responses`.

## 2. Implementation Cleanup

- [x] 2.1 Remove Grok-only WS ingress routing and HTTP-bridge replay branches.
- [x] 2.2 Remove dead Grok WS cache/model helpers and native passthrough assumptions.
- [x] 2.3 Preserve generic OpenAI WS passthrough, pooling, HTTP bridge, timeouts, and current uncommitted hardening.
- [x] 2.4 Remove or update stale Grok WS configuration and documentation surfaces.

## 3. Validation

- [x] 3.1 Add handler/route tests proving Grok upgrades are rejected before handshake.
- [x] 3.2 Add paired tests proving OpenAI WebSocket ingress is unchanged.
- [x] 3.3 Retain and run Grok HTTP streaming tests for the fallback endpoint.
- [x] 3.4 Run an end-to-end Codex probe with WebSocket incorrectly enabled and verify HTTPS/SSE fallback succeeds.
- [x] 3.5 Run a Pi `sicts-openai/grok-4.5` smoke test to prove the normal HTTP path remains healthy.
- [x] 3.6 Run targeted Go tests and `openspec validate remove-grok-websocket-ingress --strict`.
