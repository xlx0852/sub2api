# Change: Add Safe Grok Same-Account Retry

## Why

Grok Responses and Chat requests currently fail over to another account after retryable upstream failures. The existing `RetryableOnSameAccount` mechanism is limited to pool-mode account settings and re-enters account scheduling, while Grok error handling may temporarily unschedule the selected account before the retry occurs. This means a nominal same-account retry is not guaranteed to reuse the same account and can unnecessarily lose prompt-cache and conversation affinity.

## What Changes

- Add a conservative Grok same-account retry policy for HTTP Responses and Chat Completions
- Retry the already selected account directly before account failover
- Default to one retry for transient `429`, `502`, `503`, `504`, and `529` responses
- Respect short `Retry-After` delays within the request deadline and use a bounded fallback delay
- Delay Grok account health penalties until same-account retries are exhausted
- Prohibit retries after downstream output is committed or when the client context is canceled
- Keep Grok media generation/editing, video status, WebSocket ingress, `401`, and `403` outside this change
- Add configuration, error classification, retry-order, output-safety, and failover regression tests

## Impact

- Affected specs: `grok-safe-same-account-retry`
- Affected code: gateway configuration, Grok upstream error classification, Responses/Chat handler failover loops, Grok account health handling, and tests
- Preserved behavior: media APIs, WebSocket routing, non-Grok platforms, billing semantics, account concurrency limits, sticky routing, and eventual account failover
- No database migration or frontend change
