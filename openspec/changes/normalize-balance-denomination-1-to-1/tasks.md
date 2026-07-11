## 1. Preparation

- [ ] 1.1 Re-run the production inventory and compare it with the baseline in `proposal.md`.
- [ ] 1.2 Generate a read-only dry-run report containing old/new sums and representative user purchasing-power calculations.
- [ ] 1.3 Draft and review the exact transactional migration SQL; do not execute it.
- [ ] 1.4 Add automated tests proving recharge credit and user debit both scale by the same factor.
- [ ] 1.5 Prepare the customer announcement and support reconciliation export format.

## 2. Cutover Preconditions

- [ ] 2.1 Select a low-traffic maintenance window and obtain explicit operator approval.
- [ ] 2.2 Disable new recharge orders and confirm there are no in-flight payment/refund states.
- [ ] 2.3 Confirm there are no running batch jobs or unsettled balance holds.
- [ ] 2.4 Create and verify a complete PostgreSQL plus application/config backup.
- [ ] 2.5 Stop application writes and prove row counters remain stable.

## 3. Migration

- [ ] 3.1 Execute the reviewed migration in one PostgreSQL transaction under an advisory lock.
- [ ] 3.2 Set the recharge multiplier to `1` and convert all customer-denominated values by `10`.
- [ ] 3.3 Invalidate targeted billing/auth/quota caches.
- [ ] 3.4 Start a new application instance while keeping public traffic and recharge disabled.

## 4. Verification and Release

- [ ] 4.1 Reconcile all migrated aggregates against the preflight snapshot.
- [ ] 4.2 Verify representative positive, negative, API-key and affiliate balances.
- [ ] 4.3 Run one low-cost request per active standard group and verify purchasing power is unchanged.
- [ ] 4.4 Complete a small 1:1 recharge canary and verify the credited amount.
- [ ] 4.5 Switch traffic, publish the customer notice and monitor billing errors.
- [ ] 4.6 Retain the old application bundle and database backup for rollback.
