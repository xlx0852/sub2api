## 1. Catalog and metadata

- [x] 1.1 Extend model catalog schema for Codex capability metadata.
- [x] 1.2 Add official Astra metadata to both embedded catalog consumers.
- [x] 1.3 Add catalog validation for Astra limits and reasoning levels.

## 2. Codex runtime

- [x] 2.1 Add GPT-6 Astra instructions and model-specific selection.
- [x] 2.2 Add local Astra fallback manifest for discovery outages.
- [x] 2.3 Preserve `ultra`, Responses Lite, verbosity, tool mode, and multi-agent metadata through catalog and routing capability declarations.

## 3. WebSocket and replay

- [x] 3.1 Verify proactive 55-minute rotation does not break ordinary turns.
- [x] 3.2 Verify rotation/reconnect with `previous_response_id` and `function_call_output`.
- [x] 3.3 Verify compact and HTTP bridge fallback behavior.

## 4. Verification

- [x] 4.1 Run targeted Go tests and catalog tests.
- [x] 4.2 Run frontend tests and build/typecheck.
- [ ] 4.3 Run `git diff --check` and OpenSpec strict validation (OpenSpec CLI unavailable in this environment).
