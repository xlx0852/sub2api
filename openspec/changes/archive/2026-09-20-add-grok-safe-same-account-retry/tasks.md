## 1. Configuration and classification

- [x] 1.1 Add Grok same-account retry configuration, defaults, and validation
- [x] 1.2 Classify configured transient Grok Responses/Chat HTTP failures as same-account retryable
- [x] 1.3 Preserve existing behavior for authentication, policy, client, media, and WebSocket failures

## 2. Retry sequencing

- [x] 2.1 Add exact-account retry state before normal account failover
- [x] 2.2 Keep account concurrency acquisition and release balanced across attempts
- [x] 2.3 Respect bounded `Retry-After`, request cancellation, and downstream-output safety
- [x] 2.4 Apply Grok temporary unscheduling only after the retry budget is exhausted

## 3. Verification

- [x] 3.1 Add Responses and Chat exact-account retry tests
- [x] 3.2 Add retry exhaustion, account switch, output committed, cancellation, and excluded-status tests
- [x] 3.3 Run gofmt, focused tests, full affected-package tests, and server compilation
- [x] 3.4 Run strict OpenSpec validation and scoped `git diff --check`
