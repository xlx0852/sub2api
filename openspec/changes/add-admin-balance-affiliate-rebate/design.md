## Context

The admin balance endpoint already calculates the actual balance delta and records it as an `admin_balance` redeem-code history entry. Affiliate rebates are centralized in `AffiliateService.AccrueInviteRebate`, which applies all policy checks.

## Decisions

- Call `AffiliateService.AccrueInviteRebate` only when the final balance delta is positive.
- Pass the positive delta as the recharge base amount; do not create a synthetic payment order.
- Treat rebate accrual as best-effort after the balance adjustment, matching existing redeem-code behavior: a rebate failure is logged without rolling back a successful balance change.

## Risks / Trade-offs

- Direct service-level retries without the HTTP idempotency key could accrue twice because manual adjustments have no payment order ID. The admin endpoint remains protected by its existing idempotency middleware; a future ledger source identifier can strengthen direct-call deduplication.
