# Change: Add Kimi quota queries with automatic refresh

## Why

Kimi OAuth accounts expose official 5-hour and weekly quota windows, but the
management UI currently has no server-side quota source for them. This makes
Kimi accounts look empty even though the upstream API provides the data.

## What Changes

- Query Kimi's coding usage endpoint from the backend using the account's OAuth
  credentials and device headers.
- Parse the 5-hour and weekly windows into the existing `UsageInfo` contract,
  retrying once after a 401 with a refreshed Kimi token.
- Attach Sub2API's local request/token/cost statistics to both windows so the
  drawer can show request counts and full-utilization projections.
- Cache successful Kimi quota results for 10 minutes and coalesce concurrent
  requests; a forced refresh bypasses the cache.
- Render Kimi OAuth quota windows in the existing account usage cell/drawer and
  add service/UI regression coverage.
- Synchronize full Kimi windows with temporary scheduling blocks that expire at
  the official reset, and refresh quota automatically after gateway 429s.

## Impact

- Affected specs: account usage reporting (new capability)
- Affected code: backend account usage service, Kimi OAuth/upstream client,
  generated dependency wiring, and the admin usage cell.
- No database schema or public API endpoint changes are required.
