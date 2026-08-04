# Change: Add Grok Automatic Usage Sync

## Why

OpenAI OAuth usage automatically refreshes when its persisted snapshot is missing or stale, while Grok OAuth usage can retain an old persisted billing snapshot and requires an operator to click Refresh quota. This makes account usage freshness inconsistent across platforms and leaves Grok quota-based scheduling dependent on manual intervention.

The current Grok Build source confirms that the authoritative CLI OAuth control-plane path is `cli-chat-proxy /billing?format=credits`: it carries `creditUsagePercent` and `currentPeriod`, is authenticated by the CLI OAuth Bearer token plus CLI identity headers, and is polled by the official client. Sub2API must align its compatible control-plane behavior without copying browser SSO cookies, TUI-only polling, or consumer auto-topup UX.

## What Changes

- Automatically probe Grok billing when an OAuth account usage snapshot is missing or stale.
- Reuse a bounded freshness TTL and per-account singleflight so concurrent account-list reads do not fan out duplicate upstream billing requests.
- Keep manual refresh as a force operation that bypasses freshness checks.
- Fall back to the last persisted billing/header snapshot when an automatic probe fails.
- Make the successful `billing?format=credits` response authoritative for the active weekly/monthly window; use legacy `/billing` only as a compatibility fallback when credits cannot be read.
- Send the current Grok Build-compatible CLI identity headers, including client mode and version, on all billing probes.
- Preserve the official zero-value semantics for `creditUsagePercent`, `currentPeriod`, subscription tier, and reset timestamps instead of merging stale legacy fields over a fresh credits response.
- Preserve local usage statistics, billing, scheduling, and gateway forwarding behavior.
- Align Grok local request/token/cost statistics to the official weekly period
  and expose the same full-utilization projection already used by OpenAI.
- Synchronize a full official weekly Billing window with a temporary scheduling
  block ending at `period_end`, and refresh Billing after gateway 429s.

## Impact

- Affected specs: `account-usage-auto-sync`
- Affected code: account usage service, Grok quota service integration, account usage UI, dependency injection, targeted service/handler/frontend tests
- External traffic: at most one automatic credits request per account per freshness window; legacy billing is contacted only after a credits failure
- No database schema or public API response-shape change
