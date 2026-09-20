# Change: Remove Grok WebSocket Ingress

## Why

Grok text requests are stable through the HTTP Responses/SSE path, while accepting downstream Responses WebSocket connections forces Sub2API to reconstruct multi-turn state through a WS-to-HTTP bridge. That bridge can hang, replay incomplete context, or surface `Incomplete response stream` even though the same account succeeds over HTTP/SSE. Clients can also enter this path accidentally by setting `supports_websockets = true`.

## What Changes

- **BREAKING** Stop advertising or accepting Responses WebSocket ingress for Grok groups.
- Reject a Grok WebSocket upgrade before the `101 Switching Protocols` handshake with a deterministic unsupported-transport response so fallback-capable clients retry the normal HTTP Responses endpoint.
- Keep Grok `POST /responses` and `POST /v1/responses` streaming behavior unchanged.
- Remove the Grok-specific WS-to-HTTP replay path and its native Grok WebSocket routing assumptions.
- Preserve OpenAI WebSocket ingress, pooling, passthrough, HTTP bridge, and related configuration.
- Add regression coverage proving a WebSocket-enabled Codex client falls back to HTTP/SSE when it targets a Grok group.

## Impact

- Affected specs: `grok-xai-ws-passthrough` (superseded), `grok-http-stream-fallback`
- Affected code: OpenAI-compatible gateway route dispatch, Responses WebSocket handler, Grok-specific WS bridge branches/tests, provider integration guidance
- Unaffected: Grok HTTP Responses, Chat Completions compatibility, Messages compatibility, media endpoints, OpenAI WebSocket behavior

