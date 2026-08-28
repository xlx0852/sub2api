# Change: Optimize Settlement Reconciliation Scans

## Why

Production PostgreSQL repeatedly scans the full `usage_settlement_attempts` table for overdue `started` rows. The durable settlement worker runs both reconciliation queries on its one-second poll and on request-driven wakeups, while the existing indexes only cover claimable or already-reconciled states. At the current table size, empty scans consume hundreds of milliseconds and sustain unnecessary database CPU and I/O.

## What Changes

- Add an online partial index for `started` settlement attempts ordered by reconciliation deadline.
- Separate fast settlement draining from slow overdue reconciliation scheduling.
- Keep request-driven wakeups low-latency without running overdue scans for every completed request.
- Run reconciliation once at worker startup and then at a bounded periodic interval.
- Add regression tests for wakeup isolation, periodic reconciliation, and migration execution mode.

## Impact

- Affected specs: `usage-settlement-durability`
- Affected code: durable settlement worker, settlement migration, worker and migration tests
- Database: one small partial index created concurrently without blocking normal writes
- Compatibility: settlement authority, recovery ordering, leases, retry behavior, and billing semantics remain unchanged
