# Change: Update Codex Compact Health Routing And V2 SSE Contract

## Why

Production Codex clients report `stream closed before response.completed` / `remote compaction v2 stream closed before response.completed` during auto compact, while the gateway may still finish upstream work and return 200 after tens of seconds.

Earlier assumptions treated remote compact as one long-running non-streaming `/responses/compact` class and focused on account health + soft-timeout retry. Official Codex source shows a protocol split:

1. **Legacy remote compact**: path-based `POST /responses/compact`, unary JSON `{output:[...]}`.
2. **Remote compaction v2 (current default)**: ordinary `POST /responses` with `stream=true`, `input` ending in `{type:"compaction_trigger"}`, waiting for SSE `response.output_item.done` (`item.type=compaction`) then `response.completed`.

Evidence from official Codex (`openai/codex`):

- Client path: `codex-rs/core/src/compact_remote_v2.rs` (`collect_compaction_output`, around the stream wait for `OutputItemDone` + `Completed`).
- Request shape: `codex-rs/core/src/compact_remote_v2_attempt.rs` appends `ResponseItem::CompactionTrigger` and calls the normal Responses stream API (`/v1/responses`, not `/responses/compact`).
- Default on: `2026-06-11` `core: enable remote compaction v2 by default` (`Feature::RemoteCompactionV2`, `default_enabled: true`).
- Harder to avoid: `2026-06-24` `Remove auto-compaction opt-out`.

Current sub2api body-signal handling rewrites v2 requests to `/responses/compact`, strips `stream`, and serves non-stream JSON. That breaks the Codex v2 downstream SSE contract: the client keeps waiting for stream terminal events, reconnects under load, and surfaces the stream-closed error even when upstream eventually succeeds.

Soft-timeout and compact health scoring remain useful, but they are not the primary fix for v2.

## What Changes

### Protocol (primary)

- Distinguish two compact inbound shapes and keep separate downstream contracts:
  - **Path-based legacy** `POST .../responses/compact` → keep unary JSON compatibility.
  - **Body-signal v2** `POST .../responses` with `compaction_trigger` → keep path as `/responses`, keep stream semantics for the Codex client.
- Prefer native upstream `/responses` streaming for body-signal v2 when the selected account/upstream supports it.
- When upstream can only serve legacy `/responses/compact` JSON, add a **compact-only SSE bridge**:
  - Downstream: `text/event-stream` with valid SSE keepalives while waiting.
  - On success: emit `response.output_item.done` with one `type=compaction` item, then `response.completed`.
  - On failure: emit a stream-compatible error path without pretending success.
- Do not strip `stream` for body-signal v2 just to force the legacy compact path.
- Do not apply the SSE bridge or body-signal rewrite to ordinary non-compact `/responses` traffic.

### Health / ops (secondary, keep)

- Keep compact as an explicit request class for scheduling, logging, and metrics.
- Keep compact-local account penalties (soft timeout, client disconnect, high compact latency).
- Keep at most one bounded pre-commit soft-timeout retry where protocol-safe (especially before any SSE terminal event is committed).
- Keep compact outcome fields that separate upstream success from client delivery / disconnect.

## Impact

- Affected specs: `codex-compact-routing`
- Affected code (implementation phase, after approval):
  - `backend/internal/handler/openai_gateway_handler.go` body-signal promotion
  - `backend/internal/service/openai_compact_body_signal.go` and request-body normalization
  - OpenAI forward/response handling for compact SSE bridge
  - compact tests that currently assert path rewrite + stream strip for body-signal
  - optional account scheduling diagnostics already added by this change
- Compatibility:
  - Official Codex remote compaction v2 clients regain SSE terminal events.
  - Legacy path-based `/responses/compact` JSON clients stay JSON.
  - Ordinary sync/stream/WS `/responses` without `compaction_trigger` stay unchanged.
  - Non-Codex platforms stay unchanged.

## Non-Goals

- Do not convert all `/responses` traffic to a special compact mode.
- Do not require every upstream account to implement v2 natively if bridge can preserve the client contract.
- Do not reintroduce client-side auto-compact opt-out as a product dependency.
- Do not expand soft-timeout retries beyond one extra attempt.
