## Context

OpenAI Status exposes a public JSON summary containing page metadata, an aggregate indicator, component states and incident updates. The endpoint is useful operational evidence, but it is not documented as a stable OpenAI product API and its own page states that aggregate availability may differ by tier, model and feature. Sub2API already has an Ops dashboard, alert events, job heartbeats, PostgreSQL persistence and Redis-backed leader-election patterns.

## Goals / Non-Goals

- Goals:
  - Surface OpenAI official aggregate status next to Sub2API's own traffic and error evidence.
  - Preserve status transitions so operators can correlate official incidents with local error windows.
  - Make collection safe in multi-instance deployments and harmless when the external source is slow, unavailable or changes shape.
  - Keep official status freshness and provenance visible.
- Non-Goals:
  - Do not use official status to pause accounts, change routing, suppress failover, waive billing or alter client responses.
  - Do not claim that an official operational state proves every account, region, model or endpoint is healthy.
  - Do not proxy the raw external endpoint directly to browsers.
  - Do not poll once per dashboard refresh or once per user session.

## Decisions

### Dedicated collector outside the gateway hot path

Add an `OpenAIStatusCollectorService` with the same start/stop lifecycle and Redis leader-lock pattern used by existing Ops background jobs. The default interval is 60 seconds, configurable within a bounded range. Each request uses a short timeout, a maximum response-body size and a fixed HTTPS host allowlist. Redirects to a different host are rejected.

Alternatives considered:

- Browser-side polling: rejected because every open dashboard would amplify traffic and CORS/schema changes would directly break the UI.
- Fetching during every Ops API request: rejected because it couples dashboard latency and availability to the external source.
- Reusing gateway health probes: rejected because official aggregate status and account-level reachability are different signals.

### Change-only PostgreSQL snapshots plus job heartbeat

Create `provider_status_snapshots` with provider, source URL, aggregate indicator/description, normalized components JSON, normalized incidents JSON, source update time, fetch time and content hash. Insert only when the normalized content hash changes. A uniqueness constraint makes duplicate collectors harmless. Reuse `ops_job_heartbeats` for last attempt/result/error visibility.

The latest successful snapshot is the source of truth returned by admin APIs. `fresh`, `stale` and `unavailable` are derived from fetch time and configured poll interval; fetch failures never erase the last known-good snapshot.

Alternatives considered:

- Persist every poll: rejected due to unnecessary write volume.
- Redis-only cache: rejected because incident correlation and restart-safe history would be lost.
- One normalized table per external component and incident update: deferred until more providers require cross-provider querying.

### Tolerant normalization

Parsing requires only the fields used by the UI. Unknown fields are ignored. Missing optional arrays become empty. Invalid aggregate status or malformed required page metadata fails the poll without replacing the last known-good snapshot. Raw credentials and request payloads are not involved; stored public payload fields are size bounded.

### Admin-only presentation and correlation

Add admin Ops endpoints for current status and change history. The current response includes source URL, source update time, fetched time, freshness, aggregate status, selected component states and active incidents. The dashboard displays provenance and opens the official source in a new tab.

Error-trend correlation is visual only: official incident windows are overlaid on the same time axis. Existing error records remain unchanged. Operators see both labels when official status is degraded and local metrics are healthy, or vice versa.

### Alert transitions without request-path side effects

When the normalized aggregate state crosses between operational and degraded states, create or resolve an Ops alert-style event with provider status context. Deduplicate by provider and external incident/state fingerprint. Notification uses existing Ops notification settings and cooldown behavior. It never invokes scheduler/account mutation APIs.

## Risks / Trade-offs

- The public JSON schema can change -> tolerant parser, fixtures, body limit, last-known-good fallback and visible stale state.
- Official status can lag or be too aggregate -> UI labels it as official aggregate evidence and never treats it as local health truth.
- Multi-instance duplicate polling -> Redis leader lock plus database uniqueness.
- Provider endpoint outage can create noisy alerts -> collector failures update job heartbeat and freshness only; they do not create an OpenAI outage event.
- Long-term snapshot growth -> change-only writes and integration with existing Ops retention cleanup.

## Migration Plan

1. Add the snapshot table and indexes without changing existing rows or request tables.
2. Deploy the collector disabled behind an Ops runtime setting, then enable it after the admin APIs pass a live read-only probe.
3. Enable the dashboard card and history overlay once at least one successful snapshot exists.
4. Roll back by disabling the collector and hiding the UI; the additive table may remain without affecting the gateway.

## Open Questions

- Whether official degradation transitions should send email by default or only after an administrator enables the provider-status alert rule.
- Whether the first release should retain status snapshots for the general Ops retention period or use a dedicated retention window.
