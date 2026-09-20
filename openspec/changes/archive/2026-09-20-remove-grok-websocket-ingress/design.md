## Context

The gateway currently accepts a downstream Responses WebSocket for Grok groups, selects a Grok account, then forces the turn through an internal HTTP bridge. This means the HTTP upstream is stable but the downstream connection still inherits WebSocket session, replay, connection-lifetime, and terminal-event complexity. The current Codex client already supports falling back from a failed Responses WebSocket handshake to HTTPS.

The active `add-grok-xai-ws-passthrough` change is therefore superseded by this change. Native or bridged Grok WebSocket traffic is no longer an advertised gateway capability.

## Goals / Non-Goals

- Goals:
  - Make HTTP Responses/SSE the only Grok text transport.
  - Reject Grok WebSocket upgrades before accepting the connection.
  - Give fallback-capable clients a deterministic signal that causes an HTTPS retry.
  - Keep OpenAI WebSocket behavior unchanged.
  - Remove dead Grok-specific state replay and upstream WS routing code where it is no longer reachable.
- Non-Goals:
  - Do not remove the generic OpenAI WS-to-HTTP bridge.
  - Do not change Grok request sanitization, OAuth, cache identity, billing, or media paths.
  - Do not make arbitrary WebSocket-only clients emulate SSE inside an established WebSocket; protocol fallback requires a new HTTP request from the client.

## Decisions

- Decision: Reject by API-key group platform before `coderws.Accept`.
  - Why: Once the server returns `101`, it cannot convert that connection into an HTTP SSE response. Early rejection also avoids consuming WS ingress, user, or account concurrency slots.
  - Alternative considered: Continue accepting WebSocket and bridge each turn to HTTP.
  - Rejected because: This preserves the state/replay failure mode the change is intended to remove.

- Decision: Return HTTP `426 Upgrade Required` with OpenAI-style error type `websocket_not_supported` identifying WebSocket as unsupported for Grok.
  - Why: Codex 0.144.3 immediately retries Responses over HTTPS after this handshake status, without repeatedly retrying WebSocket first.
  - Alternative considered: Close with a WebSocket policy-violation frame after accepting.
  - Rejected because: The client has already selected WebSocket transport and may not retry over HTTPS after a post-handshake close.

- Decision: Retain the same `/responses` and `/v1/responses` POST handlers.
  - Why: Provider base URLs differ on whether they include `/v1`; both routes already normalize to the same Grok HTTP Responses service path.

- Decision: Scope the guard to Grok groups, not requested model names.
  - Why: The authenticated group determines the schedulable platform. Model-name heuristics can route the same name to different account types and are not an authorization boundary.

## Risks / Trade-offs

- Risk: A WebSocket-only client may surface the handshake error instead of retrying HTTP.
  - Mitigation: Return a clear unsupported-transport response and document that Grok providers must not advertise WebSocket support. Validate the current Codex client fallback behavior end to end.
- Risk: An early platform guard could accidentally affect OpenAI groups.
  - Mitigation: Add paired route tests proving Grok is rejected before upgrade while OpenAI still reaches the WebSocket handler.
- Risk: Removing Grok-specific bridge code overlaps current uncommitted WS hardening.
  - Mitigation: Delete only platform-Grok branches; preserve generic OpenAI timeout, replay, pool, and concurrency changes.

## Migration Plan

1. Add the pre-handshake Grok platform guard and tests.
2. Prove Codex with `supports_websockets = true` retries through HTTP/SSE.
3. Remove Grok-only branches and helpers from the generic WS ingress/HTTP bridge.
4. Remove Grok WS-specific tests and stale configuration/docs while retaining generic OpenAI WS coverage.
5. Build and run targeted handler, service, route, and Grok HTTP suites.
6. Deploy through the existing blue-green process and verify both a misconfigured WS client and a normal Pi HTTP client.

## Open Questions

- None. The requested policy is unconditional: Grok does not accept downstream WebSocket ingress.
