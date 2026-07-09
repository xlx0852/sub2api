## 1. Specification (revised assumptions)

- [x] 1.1 Validate compact request detection points and response commit boundaries in code.
- [x] 1.2 Confirm existing usage log fields and decide whether new nullable fields are required.
- [x] 1.3 Re-validate against official Codex remote compaction v2 (`compact_remote_v2`, default-on since 2026-06-11) and correct the OpenSpec root-cause model from “slow non-stream JSON only” to “v2 SSE contract mismatch + secondary latency/health issues”.
- [x] 1.4 Update proposal/design/spec to split `legacy_path` vs `body_signal_v2` contracts and require SSE terminal events for v2.

## 2. Backend health routing (already landed under prior assumption; keep)

- [x] 2.1 Mark remote compact requests with an explicit compact request class.
- [x] 2.2 Record compact payload size, upstream latency, selected account, retry count, client disconnect, write failure, and result status.
- [x] 2.3 Add short-lived compact health penalties to OpenAI account selection.
- [x] 2.4 Add one bounded pre-commit soft-timeout retry for compact requests.
- [x] 2.5 Add regression tests for compact detection, no-impact normal responses, compact penalty, and retry cap.

## 3. Frontend (already landed; keep)

- [x] 3.1 Show compact latency and disconnect hints in account performance details without expanding the account table row.

## 4. Backend protocol fix (new; do after approval)

- [x] 4.1 Stop treating body-signal v2 as “always rewrite to `/responses/compact` + strip stream” primary path.
- [x] 4.2 Classify inbound compact mode as `legacy_path` vs `body_signal_v2` and thread mode through forward/logging.
- [x] 4.3 For `body_signal_v2`, prefer native upstream `/responses` streaming passthrough when supported.
  - Note: current production upstreams still require legacy compact JSON; implementation bridges via `/responses/compact` while preserving client SSE. Native passthrough can be layered later when account capability signals exist.
- [x] 4.4 For `body_signal_v2` with legacy-only upstream, implement compact-only SSE bridge:
  - keepalives while waiting
  - map legacy JSON success to one `response.output_item.done` (`item.type=compaction`)
  - then `response.completed`
- [x] 4.5 Keep `legacy_path` `/responses/compact` on unary JSON.
- [x] 4.6 Make soft-timeout retry protocol-aware (allowed only before terminal SSE/JSON commit; keepalives are non-terminal).
- [x] 4.7 Record `compact_mode` and `bridge_used` in compact outcome logs/metrics.
- [x] 4.8 Rewrite/add tests:
  - body-signal v2 no longer asserts path rewrite + stream strip as success criteria
  - bridge emits required SSE terminal sequence
  - legacy path JSON unchanged
  - ordinary `/responses` without trigger unchanged
  - soft-timeout retry still works before terminal events

## 5. Verification

- [x] 5.1 Run targeted Go tests for OpenAI gateway compact routing (health/soft-timeout suite).
- [x] 5.2 Run frontend type/build checks for account performance display.
- [x] 5.3 Run new compact v2 SSE bridge/passthrough unit tests.
- [x] 5.4 Add WS HTTP bridge/replay usage marker without changing `request_type=ws_v2`.
- [x] 5.5 Promote 45s compact soft-timeout accounts to short-lived hard penalty behind healthy candidates.
- [ ] 5.6 Verify with official Codex client / API-key production-like long-context auto compact:
  - no `remote compaction v2 stream closed before response.completed`
  - exactly one compaction output item
  - `response.completed` received
  - soft-timeout switch metrics still sane
- [ ] 5.7 Deploy with blue-green and watch compact cancel rate, bridge ratio, and soft-timeout switches.
