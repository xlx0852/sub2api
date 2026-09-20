## 1. Persistence and contracts

- [x] 1.1 Add an additive migration for change-only provider status snapshots, uniqueness and query indexes.
- [x] 1.2 Add service models and repository methods for inserting and querying current/history snapshots.
- [x] 1.3 Extend Ops retention cleanup to remove expired provider status history without deleting the latest snapshot.

## 2. Safe status collection

- [x] 2.1 Implement a size-bounded, timeout-bounded OpenAI Status client with fixed-host redirect validation and tolerant JSON normalization.
- [x] 2.2 Implement the periodic collector, Redis leader lock, content-hash deduplication and job heartbeat reporting.
- [x] 2.3 Add bounded runtime settings for enabled state, polling interval and stale threshold.
- [x] 2.4 Wire collector start/stop into the server lifecycle without adding any gateway hot-path dependency.

## 3. Ops APIs and alert correlation

- [x] 3.1 Add admin-only current-status and status-history endpoints.
- [x] 3.2 Derive `fresh`, `stale` and `unavailable` states from persisted data and collector heartbeat.
- [x] 3.3 Create and resolve deduplicated provider-status observability events on official state transitions without mutating request error attribution.
- [x] 3.4 Add backend tests for schema drift, body limits, timeout/failure fallback, duplicate collectors, state transitions and authorization.

## 4. Admin UI

- [x] 4.1 Add typed frontend Ops API clients for current status and history.
- [x] 4.2 Add an OpenAI official status card showing provenance, freshness, important components, incidents and a link to the source.
- [x] 4.3 Overlay official incident windows on the Ops error trend while keeping official and local signals visually distinct.
- [x] 4.4 Add stale/unavailable/error states, dark/light theme coverage and zh/en translations.
- [x] 4.5 Add component tests for operational, degraded, stale, unavailable and schema-partial responses.

## 5. Verification

- [x] 5.1 Run targeted backend service/repository/handler tests and race tests for the collector lifecycle.
- [x] 5.2 Run frontend component tests and type checking.
- [x] 5.3 Verify a live fetch populates one snapshot, repeated unchanged fetches do not add rows, and external failure leaves gateway traffic unaffected.
- [x] 5.4 Run `git diff --check` and `openspec validate add-openai-official-status-observability --strict`.
