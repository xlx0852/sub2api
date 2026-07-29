# Change: Add Grok Automatic Usage Sync

## Why

OpenAI OAuth usage automatically refreshes when its persisted snapshot is missing or stale, while Grok OAuth usage only reads a persisted billing snapshot and requires an operator to click Refresh quota. This makes account usage freshness inconsistent across platforms and leaves Grok quota-based scheduling dependent on manual intervention.

## What Changes

- Automatically probe Grok billing when an OAuth account usage snapshot is missing or stale.
- Reuse a bounded freshness TTL and per-account singleflight so concurrent account-list reads do not fan out duplicate upstream billing requests.
- Keep manual refresh as a force operation that bypasses freshness checks.
- Fall back to the last persisted billing/header snapshot when an automatic probe fails.
- Preserve local usage statistics, billing, scheduling, and gateway forwarding behavior.

## Impact

- Affected specs: `account-usage-auto-sync`
- Affected code: account usage service, Grok quota service integration, dependency injection, targeted service/handler tests
- External traffic: at most one automatic two-request Grok billing probe per account per freshness window
- No database schema or public API response-shape change
