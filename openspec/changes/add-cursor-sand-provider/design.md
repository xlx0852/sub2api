# Design: Native Cursor Sand Provider Integration

## Context

Sub2API aggregates diverse upstream providers into OpenAI-compatible endpoints. The newly verified capability in `grok_bot_2api_temp` demonstrated that Cursor's cloud inference endpoint (`https://api2.cursor.sh`) serves advanced models (e.g. `grok-4.5`, `claude-3-7-sonnet`, `composer-2.5`) via Connect-RPC over HTTP/1.1 or HTTP/2, authenticated by OAuth Bearer tokens and dynamic `x-cursor-checksum` anti-abuse headers.

Integrating this natively in Go eliminates Node.js sidecars, minimizes latency and resource footprint, and unifies account pooling, proxy routing, and billing.

## Goals / Non-Goals

### Goals
- Native Go implementation of `x-cursor-checksum` and Connect-RPC client.
- First-class `PlatformCursor` account support with automatic token refresh.
- Full OpenAI `/v1/chat/completions` compatibility (streaming and non-streaming) including thinking chunks and tool calls.
- Support proxy routing (residential/datacenter proxies) for upstream requests.
- PKCE web authorization workflow in Sub2API Admin UI.

### Non-Goals
- Re-implementing the complete Electron Sand agent environment (e.g., local execution daemons, filesystem sandboxes).
- Modifying unrelated upstream platforms (OpenAI, Anthropic, Gemini).

## Decisions

### Decision 1: Pure Go Connect-RPC client without heavy runtime dependencies
- *Decision*: Use Connect-RPC Go SDK (`connectrpc.com/connect`) or lightweight envelope framing over `net/http` to invoke `aiserver.v1.InferenceService/Stream`.
- *Rationale*: Connect protocol is standard HTTP POST with 5-byte envelope frames (`[flags:1B][length:4B]`) and Protobuf payloads. This keeps the binary lightweight and transparent.

### Decision 2: Automated Token Rotation via Refresh Token
- *Decision*: Implement `CursorTokenRefresher` conforming to Sub2API's `TokenRefresher` interface with a 15-minute renewal window before expiry.
- *Rationale*: Cursor access tokens expire in ~60 days, but keeping proactive refresh guarantees accounts never fail in active scheduling pools.

### Decision 3: Thinking and Tool Call Normalization
- *Decision*: Map `thinkingPart` deltas into standard reasoning delta fields / content chunks, and accumulate `toolCallPart` deltas until `isComplete` before emitting downstream OpenAI tool call objects.
- *Rationale*: Aligns with Sub2API's multi-platform responses normalization standard.

## Risks / Trade-offs

- **Upstream Checksum / Version Rotation**: Cursor may update client versions or checksum algorithms.
  - *Mitigation*: Externalize version strings (`0.27.0`) and client type (`sand`) as configurable options.
- **IP Blocking by Upstream Cloudflare**:
  - *Mitigation*: Enforce proxy configuration on Cursor accounts via Sub2API's built-in proxy pool (`ProxyID`).

## Migration Plan

- Database: Zero schema migration needed (`platform` column in PostgreSQL `accounts` and `groups` accepts new string values without DDL change).
- Rollback: Disabling or removing `PlatformCursor` accounts immediately halts Cursor traffic without touching existing channels.
