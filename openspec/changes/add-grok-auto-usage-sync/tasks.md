## 1. Service integration

- [ ] 1.1 Inject the existing Grok quota service into `AccountUsageService`.
- [ ] 1.2 Detect missing or stale Grok billing snapshots using a ten-minute freshness TTL.
- [ ] 1.3 Guard automatic probes with per-account singleflight and rebuild usage from the persisted result.
- [ ] 1.4 Degrade to the last persisted billing/header snapshot when automatic probing fails.
- [ ] 1.5 Keep the existing manual quota endpoint as the force-refresh path.

## 2. Tests

- [ ] 2.1 Cover automatic probing for missing and stale snapshots.
- [ ] 2.2 Cover suppression for fresh snapshots.
- [ ] 2.3 Cover concurrent reads issuing only one probe.
- [ ] 2.4 Cover probe failure returning stale usage.
- [ ] 2.5 Cover manual refresh bypassing freshness suppression.

## 3. Verification

- [ ] 3.1 Run targeted Go service and handler tests.
- [ ] 3.2 Run frontend account usage tests to confirm the request contract is unchanged.
- [ ] 3.3 Run OpenSpec strict validation and mark all tasks complete.
