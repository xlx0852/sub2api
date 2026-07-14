## 1. Preparation

- [x] 1.1 Re-run the production inventory and compare it with the baseline in `proposal.md`.
- [x] 1.2 Generate a read-only dry-run report containing old/new sums and representative user purchasing-power calculations.
- [x] 1.3 Draft and review the exact transactional migration SQL before production execution.
- [x] 1.4 Add automated tests proving recharge credit and user debit both scale by the same factor.
- [x] 1.5 Prepare the customer announcement and support reconciliation export format.

## 2. Cutover Preconditions

- [x] 2.1 Select a low-traffic cutover window and obtain explicit operator approval.
- [x] 2.2 Disable new recharge orders and confirm there are no in-flight payment/refund states.
- [x] 2.3 Confirm there are no running batch jobs or unsettled balance holds.
- [x] 2.4 Create and verify a complete PostgreSQL plus application/config backup.
- [x] 2.5 Verify the online high-water-mark and short write-drain strategy on a restored production clone with a concurrent balance update.

## 3. Migration

- [x] 3.1 Execute the reviewed migration in one PostgreSQL transaction under an advisory lock.
- [x] 3.2 Set the recharge multiplier to `1`, convert normal customer-denominated values by `10`, and convert the special recharge assets by `5`.
- [x] 3.3 Invalidate targeted billing/auth/quota caches.
- [x] 3.4 Start and switch to a healthy parallel instance while keeping public traffic online and recharge disabled.

## 4. Verification and Release

- [x] 4.1 Reconcile all migrated aggregates against the preflight snapshot and correct old-instance in-flight tail rows.
- [x] 4.2 Verify representative positive, negative, API-key and affiliate balances.
- [x] 4.3 Verify live post-cutover requests use the new group multiplier and no new old-denomination rows appear.
- [x] 4.4 Observe a completed production recharge canary where `50.00` principal credited `50.00` balance.
- [x] 4.5 Switch traffic, publish the popup customer notice and monitor billing errors.
- [x] 4.6 Retain the old application bundle and verified database backup for rollback.
