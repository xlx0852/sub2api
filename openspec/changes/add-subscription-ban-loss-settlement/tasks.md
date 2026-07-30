## 1. Database and repository

- [ ] 1.1 Add migrations for subscription termination and refund ledgers, constraints, indexes, reversal/void audit fields, and migration regression coverage.
- [ ] 1.2 Add service-domain types and batch repository methods for terminations, refunds, and derived cycle loss data.
- [ ] 1.3 Implement an idempotent repository transaction that locks the cycle, creates the termination, disables the account and credential shadows, and enqueues scheduler outbox events atomically.
- [ ] 1.4 Implement refund creation, refund voiding, and termination reversal with row locking, total-refund validation, and integration tests.
- [ ] 1.5 Prevent deletion of a cycle with termination/refund history and return a stable conflict error.

## 2. Accounting service

- [ ] 2.1 Extend range-cost calculation to stop regular amortization at the active ban timestamp and recognize the remaining gross cost at that timestamp.
- [ ] 2.2 Apply non-voided refunds as negative cost on their actual receipt timestamps and prove that lifecycle cost equals purchase fee minus refunds.
- [ ] 2.3 Calculate cycle revenue before ban, net purchase cost, recovered amount, recovery progress, realized profit, and realized loss.
- [ ] 2.4 Freeze current-cycle elapsed progress and revenue at the ban timestamp while preserving the original cycle end for audit display.
- [ ] 2.5 Batch-load termination/refund data for summary, trend, overview, and account drawer without per-account or per-cycle queries.
- [ ] 2.6 Add table-driven tests for no refund, initial partial refund, delayed partial refunds, fully recovered cycles, zero-fee cycles, refund overrun rejection, reversed termination, and timezone/range boundaries.

## 3. Admin API and cache behavior

- [ ] 3.1 Add handlers and routes for termination preview, termination, refund, refund void, and termination reversal with stable validation errors.
- [ ] 3.2 Extend cycle-list and account-profit response contracts with termination, refund, and derived loss summaries while preserving existing fields for unaffected cycles.
- [ ] 3.3 Invalidate overview snapshots after each successful termination/refund/reversal write and add handler cache regression tests.
- [ ] 3.4 Verify that a successful termination immediately removes the account and credential shadows from schedulable snapshots, while reversal does not auto-enable them.

## 4. Frontend

- [ ] 4.1 Extend the profit API client types and methods for termination/refund/reversal contracts.
- [ ] 4.2 Add a guarded “封禁结算” action to each eligible subscription cycle with account/cycle details, local effective time, optional received refund, derived loss preview, and a stop-scheduling warning.
- [ ] 4.3 Render terminated cycles with ban time, pre-ban revenue, refund total, net purchase cost, recovery progress, and realized loss; support adding later refunds and audited correction actions.
- [ ] 4.4 Update the account statistics drawer to show finalized ban loss/profit instead of treating a terminated cycle as an absent active cycle.
- [ ] 4.5 Add Chinese/English copy and component tests for termination confirmation, loss preview, later refund reduction, zero-loss recovered accounts, and disabled saving states.

## 5. Verification and rollout

- [ ] 5.1 Run focused backend unit/integration tests, frontend component/type tests, and `openspec validate add-subscription-ban-loss-settlement --strict`.
- [ ] 5.2 On a staging account, verify the database ledger, scheduler outbox/snapshot removal, account drawer, daily profit adjustment, and delayed-refund recalculation end to end.
