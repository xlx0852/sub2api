## 1. Backend

- [x] 1.1 Add Grok WS passthrough mode parsing and account/group-level gating.
- [x] 1.2 Implement Grok WS URL, header, and body builders.
- [x] 1.3 Add downstream/upstream response ID mapping for Grok WS turns.
- [x] 1.4 Add session-scoped Grok WS connection store with reuse and failure eviction.
- [x] 1.5 Route eligible Grok Codex WS traffic to Grok WS before HTTP bridge.
- [x] 1.6 Keep HTTP bridge fallback for non-WS, media, disabled, and auto-failure cases.
- [x] 1.7 Carry Grok WS connection and fallback metrics through usage logging.

## 2. Frontend / Admin

- [x] 2.1 Add or expose Grok WS passthrough mode controls where account/group transport settings are managed.
- [ ] 2.2 Show Grok WS fallback and connection health hints in account performance details.

## 3. Validation

- [x] 3.1 Add unit tests for Grok WS header/body builders.
- [x] 3.2 Add WebSocket tests for `previous_response_id` mapping and incremental input.
- [x] 3.3 Add fallback tests for auto mode and forced mode.
- [x] 3.4 Run targeted backend tests.
- [x] 3.5 Run frontend type check or build where feasible.
- [x] 3.6 Test sync, stream, and Codex WS with the Grok group API key.
- [x] 3.7 Blue-green deploy and enable only for the Grok group first.
