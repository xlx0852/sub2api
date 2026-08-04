# Change: Apply affiliate rebates to admin balance recharges

## Why

Admin-issued balance recharges currently update the invitee balance but bypass the existing affiliate rebate accrual path.

## What Changes

- Treat a positive admin balance adjustment as a recharge for affiliate rebate purposes.
- Reuse the existing affiliate feature flag, rate, validity, freeze, and cap rules.
- Do not accrue rebates for balance subtraction or reductions caused by a `set` operation.
- Keep the existing admin balance ledger/audit record and idempotent request behavior.

## Impact

- Affected code: admin user balance service and dependency wiring.
- Affected capability: affiliate rebates and admin balance adjustments.
