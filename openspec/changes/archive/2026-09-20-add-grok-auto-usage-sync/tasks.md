## 1. Service integration

- [x] 1.1 Align Grok CLI billing identity headers with Grok Build's current OAuth control-plane request.
- [x] 1.2 Treat a successful `billing?format=credits` response as authoritative and call legacy `/billing` only after credits failure.
- [x] 1.3 Parse and preserve zero-value `creditUsagePercent`, `currentPeriod`, subscription tier, and reset timestamp semantics without stale-field overlay.
- [x] 1.4 Inject the existing Grok quota service into `AccountUsageService`.
- [x] 1.5 Detect missing or stale Grok billing snapshots using a ten-minute freshness TTL.
- [x] 1.6 Guard automatic probes with per-account singleflight and rebuild usage from the persisted result.
- [x] 1.7 Degrade to the last persisted billing/header snapshot when automatic probing fails.
- [x] 1.8 Keep the existing manual quota endpoint as the force-refresh path.
- [x] 1.9 Query Grok local usage from the official weekly period start and attach it to the weekly quota window.
- [x] 1.10 Render Grok full-utilization request, token, account-cost, and user-charge projections through the shared OpenAI usage component.
- [x] 1.11 Pause Grok scheduling at full official weekly utilization until `period_end`, and clear the owned pause after recovery.
- [x] 1.12 Trigger a background official Billing refresh after Grok gateway 429 responses.

## 2. Tests

- [x] 2.1 Verify exact credits-first request URL and Grok Build-compatible identity headers.
- [x] 2.2 Verify a fresh zero weekly credits response cannot be overwritten by legacy billing data.
- [x] 2.3 Cover legacy billing fallback only after credits failure.
- [x] 2.4 Cover automatic probing for missing and stale snapshots.
- [x] 2.5 Cover suppression for fresh snapshots.
- [x] 2.6 Cover concurrent reads issuing only one probe.
- [x] 2.7 Cover probe failure returning stale usage.
- [x] 2.8 Cover manual refresh bypassing freshness suppression.
- [x] 2.9 Cover Grok weekly-window alignment and full-utilization projection rendering.
- [x] 2.10 Cover Billing-driven pause, recovery clearing, and 429-triggered refresh.

## 3. Verification

- [x] 3.1 Run targeted Go service and handler tests.
- [x] 3.2 Run frontend account usage tests to confirm the request contract is unchanged.
- [x] 3.3 Run OpenSpec strict validation and mark all tasks complete.
