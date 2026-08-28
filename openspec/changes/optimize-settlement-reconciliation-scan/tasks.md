## 1. Specification and diagnosis

- [x] 1.1 Confirm the production CPU owner, hot SQL, table cardinality, and missing index.
- [x] 1.2 Define the fast-drain and slow-reconciliation scheduling boundary.

## 2. Implementation

- [x] 2.1 Add an idempotent concurrent partial-index migration for overdue started attempts.
- [x] 2.2 Add a dedicated reconciliation interval and startup reconciliation pass.
- [x] 2.3 Ensure request wakeups and normal drain polling do not execute reconciliation scans.

## 3. Verification

- [x] 3.1 Add worker regression tests for startup, periodic scans, and wakeup isolation.
- [x] 3.2 Add migration execution-mode and index-definition coverage.
- [x] 3.3 Run targeted Go tests, migration tests, backend build, and strict OpenSpec validation.

## 4. Production rollout

- [x] 4.1 Back up and validate production PostgreSQL before applying the migration.
- [x] 4.2 Start and validate a blue-green API candidate before cutover.
- [x] 4.3 Verify the concurrent index and index-backed production query plans.
- [x] 4.4 Cut over and compare CPU, load, active SQL, health, and billing backlog metrics.
- [x] 4.5 Preserve rollback pointers and document residual reconciliation backlog risk.

- 2026-08-28 production verification: migration 203 created a valid 16 KB partial index. The two empty overdue predicates changed from parallel sequential scans taking 234.304 ms and 202.102 ms to index scans taking 0.051 ms and 0.028 ms. PostgreSQL CPU fell from a 136% snapshot to 14-32% under live traffic, while a later host snapshot was 96.9% idle with load average 0.78. Eighty active-query samples after cutover contained no reconciliation scans. API 0.1.346 remained healthy with zero restarts, twenty public health checks passed, no post-cutover Nginx 5xx were observed, and usage rows continued to arrive.
- Rollback remains API 0.1.345 on port 18267 with its release, container, marker, and Nginx backup retained. The 1,208 historical `reconciliation_required` rows are a separate operational backlog and were not mutated by this optimization.
