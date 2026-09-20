## Context

`GET /admin/accounts/:id/usage` already runs automatically from the account UI. OpenAI uses this path to refresh stale quota data, while Grok needs a bounded equivalent that reads and updates `grok_billing_snapshot` without turning an account-list render into unbounded upstream traffic.

Grok Build 0.2.114 uses CLI OAuth to query `cli-chat-proxy /billing?format=credits`, which forwards to the current credits-config source. Its `creditUsagePercent` and `currentPeriod` describe the live usage window; older amount fields remain only as compatibility data. The browser-only `GetGrokCreditsConfig` gRPC-Web request is not an integration target because it requires SSO and Cloudflare browser state.

## Goals / Non-Goals

- Goals:
- Keep Grok usage fresh without requiring operator interaction.
- Prevent duplicate or high-frequency probes when many UI consumers request the same account.
- Match Grok Build's CLI OAuth billing identity and credits-first source semantics.
- Preserve stale data when the upstream probe fails.
- Non-Goals:
  - Change quota scheduling weights, billing calculations, or Grok forwarding.
- Add background cron jobs, browser-cookie storage, or schema migrations.
- Automatically reset Grok quota.
- Copy Grok Build's TUI-only 30-second poll, desktop UX, or consumer auto-topup surface into the gateway.

## Decisions

- Decision: trigger automatic synchronization from `AccountUsageService.GetUsage` for Grok OAuth accounts.
  - This reuses the UI's existing lazy-loading and request queue rather than adding another frontend request path.
- Decision: use the persisted billing `FetchedAt` timestamp as the cross-process freshness source and a per-process singleflight as duplicate suppression.
  - A ten-minute freshness window matches the existing OpenAI usage refresh cadence.
- Decision: automatic probe failures degrade to the last persisted billing or rate-limit-header snapshot.
  - The usage endpoint remains usable during xAI billing outages.
- Decision: manual quota refresh remains the explicit force path.
  - Operator-triggered refresh is not blocked by the automatic TTL.
- Decision: `billing?format=credits` is the authoritative probe response when successful.
  - Rationale: Grok Build uses this exact CLI OAuth route and treats `creditUsagePercent` / `currentPeriod` as current state. A successful credits response must not be overlaid by an older legacy `/billing` result.
- Decision: mirror Grok Build CLI identity headers for billing probes, with server-side `headless` mode.
  - Rationale: the proxy uses client identity to select the current credits route; the service is a non-interactive caller, so it uses the same headless mode as official CLI single-shot paths.

## Risks / Trade-offs

- Opening an account list can create upstream traffic for stale Grok accounts.
  - Mitigation: existing frontend queueing, persisted TTL, and per-account singleflight cap duplicate work.
- A probe can take up to the existing Grok billing timeout.
  - Mitigation: return stale data on failure; only run legacy billing as one compatibility fallback after credits fails.
- Multiple application replicas may each probe once at the freshness boundary.
  - Mitigation: the persisted `FetchedAt` check suppresses subsequent probes after the first successful writer; no distributed lock is introduced in this change.

## Migration Plan

1. Inject the existing Grok quota service into account usage service.
2. Add freshness detection and singleflight guarded probing.
3. Add success, freshness, concurrency, failure-degradation, and force-refresh tests.
4. Deploy without data migration; rollback by reverting the service integration.
