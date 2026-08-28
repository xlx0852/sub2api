## Context

The worker currently uses one event loop for three different cadences: one-second fallback polling, per-request wakeups, and one-minute statistics. Every event runs provider recovery and expiry scans before draining ready settlements. Production has roughly 389,000 settlement rows but only about 140 `started` rows, and no index serves the `state = 'started' AND reconciliation_deadline <= now()` predicate.

## Goals / Non-Goals

- Goals:
  - Make overdue scans index-backed.
  - Preserve immediate draining after authoritative handoff.
  - Bound reconciliation scans independently of request rate.
  - Preserve startup recovery and existing recovery-before-expiry ordering.
- Non-Goals:
  - Change settlement authority or rollout scope.
  - Resolve historical `reconciliation_required` records.
  - Change the 24-hour reconciliation deadline.

## Decisions

- Add a partial btree index on `(reconciliation_deadline, id)` where the row is `started` and has a non-null deadline.
  - Both overdue queries can use the same small index; provider recovery fields remain residual filters over the small started set.
- Add a dedicated reconciliation interval with a one-minute default.
  - Deadlines are measured in hours, so one minute preserves behavior while removing request-rate amplification.
- Run reconciliation once when the worker starts, then only from the reconciliation ticker.
  - Poll, wake, and stats events do not run overdue scans.
- Keep settlement draining on the existing one-second poll and request wake channel.

## Risks / Trade-offs

- An overdue row may be recognized up to one minute later than before.
  - The reconciliation deadline is 24 hours, so this bounded delay is operationally immaterial.
- Concurrent index creation can take longer under write load.
  - The migration is idempotent, non-transactional, and verified before cutover.

## Migration Plan

1. Build and test the worker change and non-transactional migration.
2. Back up production PostgreSQL and validate the backup.
3. Start the candidate; its migration runner creates the partial index concurrently.
4. Verify the index, query plans, candidate health, and migration record.
5. Cut over Nginx and compare PostgreSQL/API CPU plus active-query samples.
6. Roll back traffic to the previous API container if health or billing checks fail; retain the safe additive index.
