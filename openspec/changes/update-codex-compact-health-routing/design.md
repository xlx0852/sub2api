## Context

Codex remote compact is no longer a single protocol.

### Official Codex contracts

| Mode | Client request | Client wait contract | Upstream reference |
|------|----------------|----------------------|--------------------|
| **Remote compaction v2 (default)** | `POST /v1/responses`, `stream=true`, `input` includes `{type:"compaction_trigger"}`, beta feature `remote_compaction_v2` | SSE events until `response.output_item.done` with exactly one `item.type=compaction`, then `response.completed` | Official client streams via normal Responses API (`compact_remote_v2.rs`) |
| **Legacy remote compact** | `POST /v1/responses/compact` | Unary JSON body with compacted `output` items | Official `CompactClient` unary path (`codex-api/src/endpoint/compact.rs`) |

Timeline that explains recent production pain:

- `2026-05-04` remote compaction v2 Responses path added.
- `2026-05-15` v2 uses `compaction_trigger` item.
- `2026-06-11` **v2 enabled by default**.
- `2026-06-24` **auto-compaction opt-out removed**.

### Current sub2api mismatch

Body-signal detection currently:

1. Rewrites bare `/responses` + `compaction_trigger` to `/responses/compact`.
2. Strips `stream` / `store` / `prompt_cache_key`.
3. Forces non-stream JSON handling and compact-capable account selection.

That path was introduced to fix “v2 expected exactly one compaction output item, got 0” by steering body-signal traffic onto the legacy compact endpoint. It is the wrong long-term contract for current Codex clients:

- Codex v2 still listens on a **Responses SSE stream**.
- Returning late JSON (or closing after JSON) without `response.completed` yields:
  - `remote compaction v2 stream closed before response.completed`
  - or generic `stream closed before response.completed`
- Slow upstream (47–80s) makes the disconnect obvious, but **latency is an amplifier, not the root protocol bug**.

### What already landed under the old assumption

Still valuable and should stay:

- Compact request classification / metrics.
- Compact-local scheduling penalties.
- Soft-timeout one-shot account switch before final commit.
- Client-disconnect outcome logging.
- Account performance compact hints.

Must be revised:

- “Always normalize body-signal to `/responses/compact` non-stream JSON”.
- Tests that assert stream strip + path promotion as the desired end state for v2.

## Goals / Non-Goals

Goals:

- Restore official Codex remote compaction v2 downstream SSE compatibility.
- Preserve legacy `/responses/compact` JSON compatibility.
- Prefer native upstream v2 stream when available; otherwise bridge legacy JSON compact into v2 SSE shape.
- Keep compact health scoring and bounded soft-timeout retry as secondary reliability tools.
- Keep ordinary non-compact `/responses` behavior unchanged.

Non-Goals:

- Do not invent a third compact wire format.
- Do not make soft-timeout retry unbounded.
- Do not depend on users disabling auto compact.
- Do not change non-OpenAI platforms in this change.

## Decisions

### Decision: Split compact modes by inbound shape

Detect and mark:

1. `compact_mode=legacy_path` when URL is already `.../responses/compact`.
2. `compact_mode=body_signal_v2` when URL is bare `.../responses` and input contains `compaction_trigger`.

Scheduling/logging may still treat both as “compact class”, but **response serialization follows mode-specific contracts**.

Why: one code path for detection, two contracts for wire format.

### Decision: Stop promoting body-signal v2 onto legacy path as the primary strategy

Body-signal v2 MUST remain on `/responses` from the client-facing path perspective.

Upstream selection:

1. **Preferred**: forward as streaming `/responses` to upstreams that understand `compaction_trigger` / remote compaction v2.
2. **Fallback**: call legacy `/responses/compact` upstream, then **SSE-bridge** the result back to the Codex v2 client contract.

Why: official Codex default is v2 stream; rewriting away stream semantics is the regression.

### Decision: Compact-only SSE bridge for legacy upstreams

When `compact_mode=body_signal_v2` and upstream returns legacy compact JSON:

Downstream must:

1. Commit SSE headers early enough to start the stream.
2. Send valid SSE keepalives while waiting (JSON-comment or protocol-safe ping events already used elsewhere, without polluting normal `/responses` helpers unless reused carefully).
3. On success, emit:
   - `response.output_item.done` with exactly one item:
     - `type: "compaction"`
     - carry encrypted/summary content derived from legacy compact output
   - then `response.completed` with response id + usage if available
4. If multiple legacy output items exist, map them into the single compaction item expected by v2, or fail closed with a clear stream error if mapping is impossible.
5. Never end the stream without either `response.completed` or an explicit stream error event/close after error signaling.

Why: Codex `collect_compaction_output` hard-requires `saw_completed` and exactly one compaction output item.

### Decision: Soft-timeout retry remains, but protocol-aware

Soft-timeout retry is still allowed **before any irreversible downstream terminal event**:

- Legacy JSON mode: before response body commit (existing behavior).
- Body-signal v2 SSE mode: before emitting `response.output_item.done` / `response.completed`. Keepalives alone must not block retry if no terminal event was sent.

Why: health routing still reduces long-tail slow accounts; protocol correctness is orthogonal.

### Decision: Keepalive is compact-scoped

Do not enable generic `/responses` keepalive changes as the fix.

Use compact-only keepalive/bridge logic so normal streaming and WSv2 paths are not regressed.

### Decision: Preserve diagnostics

Continue recording:

- compact mode (`legacy_path` vs `body_signal_v2`)
- payload size, account, latency, retry count
- whether bridge was used
- client disconnect vs upstream success
- whether terminal SSE events were written

## Alternatives Considered

### A. Only soft-timeout + faster accounts

Rejected as primary fix. Faster accounts reduce frequency but still break clients when any compact exceeds proxy/client patience without SSE terminal events.

### B. Always force legacy `/responses/compact` and ask users to disable v2

Rejected. Official default is v2; opt-out was removed.

### C. Always synthesize SSE even when upstream already streams v2

Rejected as default. Prefer passthrough of native upstream SSE when protocol-compatible; bridge only for legacy upstream JSON.

### D. Return JSON for v2 and rely on client tolerance

Rejected. Official client explicitly errors if stream ends without `response.completed`.

## Risks / Mitigations

- **Risk**: Bridge maps legacy compact JSON incorrectly into `type=compaction` item.
  - Mitigation: fixture tests from official Codex SSE shapes (`response.output_item.done` + `response.completed`); fail closed if output shape is ambiguous.
- **Risk**: Early SSE headers prevent account switch after soft-timeout.
  - Mitigation: allow retry only before terminal events; keepalives are non-terminal.
- **Risk**: Native v2 upstream support is uneven across OAuth/API-key accounts.
  - Mitigation: capability detection + bridge fallback; metrics on bridge vs native ratio.
- **Risk**: Existing body-signal tests encode the wrong desired state.
  - Mitigation: rewrite tests first to lock v2 SSE contract, then implement.
- **Risk**: Double-billing on soft-timeout retry.
  - Mitigation: keep single retry cap and record both attempts.

## Rollout

1. Update OpenSpec assumptions and acceptance scenarios (this revision).
2. Implement detection split without changing external behavior flags if needed for staged rollout.
3. Implement body-signal v2 passthrough / SSE bridge behind clear code paths and tests.
4. Keep compact health + soft-timeout enabled.
5. Verify with official Codex client against long-context auto compact:
   - no `remote compaction v2 stream closed before response.completed`
   - one compaction output item
   - `response.completed` observed
6. Blue-green deploy; watch compact cancel rate, bridge ratio, soft-timeout switch rate.

## Open Implementation Notes

- Exact mapping from legacy compact `output[]` → single v2 `compaction` item must follow official replacement-history behavior and current production payloads; lock with golden fixtures during implementation.
- Confirm whether ChatGPT OAuth compact endpoint still only supports legacy JSON; if yes, bridge will be the common path for OAuth.
- Confirm header requirements (`x-codex-beta-features: remote_compaction_v2`, turn metadata) for native upstream v2 passthrough.
