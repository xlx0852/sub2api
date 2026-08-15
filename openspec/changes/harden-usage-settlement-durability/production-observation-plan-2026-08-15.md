# Production observation gate — prepared 2026-08-15

This is an execution checklist, not authorization to access or mutate production. The United States production environment must not be contacted until the owner explicitly approves the canary and identifies the intended host/load-balancer scope.

## Entry conditions

- Deploy only the clean candidate whose Linux/amd64 SHA256 is `512074376b34ea68d0fbb8c718d5c0bcf3fe61e14d266e1a1bd5731aade1e20c`; deploy its unchanged matching frontend as a pair.
- Preserve the immediately previous backend image and matching frontend artifact.
- Verify migrations 195/196/197 and confirm pre-existing rows are `authoritative=false`.
- Start the entire new-version pool in `off`; confirm login, API-key auth, health, ordinary requests, and the invariant query before enabling journal writes.
- Choose a dedicated low-risk positive group ID. Production must not use `*`.
- Capture per-test-identity balance, API-key quota, applicable account quota/outbox state, usage count, dedup count, and settlement count before traffic.

## Promotion sequence

1. Set `shadow` with only the test group, restart the intended pool deliberately, and record every instance's effective startup log.
2. Send selected and unselected controls for every provider and supported transport in that cohort. Reconcile shadow payloads against legacy usage and prove there is no second charge.
3. Stop if any fingerprint conflict, unexplained response/usage difference, retryable/terminal failure, or reconciliation row appears.
4. Set `authoritative` with the same group and restart deliberately. Before new traffic, prove historical shadow rows remain `authoritative=false`, `ready`, and unclaimed.
5. Send a bounded canary. For every identity, prove exactly one authoritative settled row, one usage log, one current-or-archived billing dedup identity, and exact balance/quota deltas.
6. Include client disconnect and one controlled instance termination only after ordinary requests pass; confirm lease recovery and no duplicate charge.
7. Return to `off` immediately on any invariant breach. Remove traffic, stop claims, allow processing to drain or leases to expire, then deploy the preserved old backend/frontend pair if rollback is required.

## Observation evidence required before adding another group

- Zero rows from the settled/usage/dedup invariant query.
- Zero unexplained `processing`, `failed_retryable`, `failed_terminal`, or `reconciliation_required` rows.
- Historical shadow count and state unchanged.
- Selected balance and quota deltas equal summed authoritative payload cost at database precision.
- Unselected controls create no journal row and retain exactly-once legacy billing.
- No protocol schema, streaming termination, scheduling/failover, login/auth, Profit, or cache/notification regression attributable to the candidate.
- A tested paired rollback remains available.

## Final deletion and archive gate

Do not remove the legacy worker-pool/`RecordUsage` commit branches while any real group can run in `off` or outside the authoritative scope. Deletion additionally requires successful authoritative evidence for Grok, Antigravity, Gemini, all active production groups, and every billable short path. The post-removal semantics of `off` must be specified explicitly because the current behavior-preserving change promises `off` as the default.

Do not archive `harden-usage-settlement-durability` until the owner declares the production observation period complete and all tasks are checked with attached evidence.
