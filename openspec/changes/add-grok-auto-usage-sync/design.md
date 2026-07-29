## Context

`GET /admin/accounts/:id/usage` already runs automatically from the account UI. OpenAI uses this path to refresh stale quota data, but Grok currently calls only `GrokQuotaFetcher.BuildUsageInfo`, which reads `grok_billing_snapshot` and `grok_usage_snapshot` from account extra data. The active Grok billing probe exists separately behind the manual quota endpoint and performs two upstream billing requests.

## Goals / Non-Goals

- Goals:
  - Keep Grok usage fresh without requiring operator interaction.
  - Prevent duplicate or high-frequency probes when many UI consumers request the same account.
  - Preserve stale data when the upstream probe fails.
- Non-Goals:
  - Change quota scheduling weights, billing calculations, or Grok forwarding.
  - Add background cron jobs or schema migrations.
  - Automatically reset Grok quota.

## Decisions

- Decision: trigger automatic synchronization from `AccountUsageService.GetUsage` for Grok OAuth accounts.
  - This reuses the UI's existing lazy-loading and request queue rather than adding another frontend request path.
- Decision: use the persisted billing `FetchedAt` timestamp as the cross-process freshness source and a per-process singleflight as duplicate suppression.
  - A ten-minute freshness window matches the existing OpenAI usage refresh cadence.
- Decision: automatic probe failures degrade to the last persisted billing or rate-limit-header snapshot.
  - The usage endpoint remains usable during xAI billing outages.
- Decision: manual quota refresh remains the explicit force path.
  - Operator-triggered refresh is not blocked by the automatic TTL.

## Risks / Trade-offs

- Opening an account list can create upstream traffic for stale Grok accounts.
  - Mitigation: existing frontend queueing, persisted TTL, and per-account singleflight cap duplicate work.
- A probe can take up to the existing Grok billing timeout.
  - Mitigation: return stale data on failure and do not introduce retries beyond the existing two billing variants.
- Multiple application replicas may each probe once at the freshness boundary.
  - Mitigation: the persisted `FetchedAt` check suppresses subsequent probes after the first successful writer; no distributed lock is introduced in this change.

## Migration Plan

1. Inject the existing Grok quota service into account usage service.
2. Add freshness detection and singleflight guarded probing.
3. Add success, freshness, concurrency, failure-degradation, and force-refresh tests.
4. Deploy without data migration; rollback by reverting the service integration.
