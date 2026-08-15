# Architecture and deployment matrix

Sub2API is a **modular monolith**: one Go binary, one Vue console, PostgreSQL + Redis. It is not a microservice split. Protocol I/O, scheduling, and billing semantics stay stable across refactors.

## Backend layout

Dependency direction (CI: `backend/internal/archtest` + golangci depguard):

`domain` → `ports` → `app` / `service` → `handler` / `repository`

| Package | Role |
|---------|------|
| `internal/domain` | Entities, constants, errors. No `internal` imports. |
| `internal/ports` | Repository / cache / upstream interfaces. Imports `domain` only. |
| `internal/app` | Extracted application modules: `gateway/pipeline`, `scheduling`, `billing`, `lifecycle`. Must not import `service` / `repository` / `handler`. |
| `internal/service` | Remaining business logic and adapters that implement `app` ports. |
| `internal/repository` | Ent / HTTP / OAuth implementations of `ports`. Must not import `service`. |
| `internal/handler` + `internal/server` | HTTP transport, routes, middleware. |
| `internal/config` | Viper load. YAML/env **key names unchanged**. Module accessors live in `modules.go`. |
| `internal/web` | Optional `-tags embed` static file server. No HTML settings injection. |

`repository` must not import `service`. New package-level `Set*` assembly bypasses are forbidden.

## Gateway pipeline

Inbound traffic is orchestrated as:

Parse → Admission → Select → Rewrite → Transport → Relay → Settle

Provider-specific protocol work stays in explicit adapters (Claude, OpenAI/Codex, Gemini, Antigravity, Grok, Kimi). Shared stages own scheduling, admission, relay, and settlement. Entry short-circuits (passthrough, Bedrock, raw Grok/Kimi chat completions, Codex CLI-only) remain outside the pipeline.

`AccountSelector` (`app/scheduling`) is the production selection entry. Two scheduler implementations still exist behind adapters; wait-plan / slot / lookup helpers are shared.

`app/billing.Settlement` is the usage settlement facade. Charge cores are still the existing implementations.

Wire + `app/lifecycle.Registry` own start/stop. `cmd/server/cleanup.go` is generated from the registry.

## Durable usage settlement (rollout gated)

Terminal usage can be handed off through the additive PostgreSQL table `usage_settlement_attempts`. The durable path does not replace either provider-specific cost calculator: it persists their existing scalar billing command and usage-log snapshot, then commits billing deduplication, balance/quota effects, the usage log, and the terminal `settled` state in one transaction.

`started` → `ready` → `processing` → `settled`

Alternative terminal states are `failed_retryable`, `failed_terminal`, and `reconciliation_required`. A processing lease is reclaimable after expiry; Redis and the in-process queue are wake-up accelerators only. Post-commit cache refreshes and notifications are intentionally outside the accounting transaction and are rebuildable from PostgreSQL.

The startup setting `usage_settlement_durability_mode` has three values:

| Mode | Journal writes | Durable claims | Billing authority |
|------|----------------|----------------|-------------------|
| `off` (default) | No | No | Existing non-durable path |
| `shadow` | Yes | No | Existing path; journal is comparison evidence only |
| `authoritative` | Yes | Yes | PostgreSQL durable worker |

For `shadow` or `authoritative`, the second database setting `usage_settlement_durability_group_ids` is mandatory. It accepts a JSON array of positive group IDs; explicit `*` is reserved for isolated whole-instance testing. The same scope controls handler handoff, journal writes, legacy-path suppression, and PostgreSQL claims. Unselected groups create no journal row and retain the existing worker-pool billing path. Missing, unreadable, empty, malformed, or non-positive active scope fails closed to `off`.

Each attempt also records immutable claim eligibility at creation. Shadow and pre-migration rows have `authoritative=false` and remain comparison evidence forever; switching the same group to authoritative does not upgrade or claim them. Only attempts created while that group is authoritative have `authoritative=true` and can be claimed. Group scope is an additional filter, not a replacement for this per-attempt guard.

Both settings are read once when an instance starts. They are not YAML/env keys and do not change the default deployment. Production rollout should start with one low-risk test group; `*` must not be used as a production canary shortcut.

The default reconciliation window is 24 hours. Exact pre-terminal recovery is conditional: only official OpenAI API-key and xAI API-key stored Responses with `store=true` can be retrieved by response ID. `store=false`, OAuth/Codex, Chat Completions, Anthropic, Gemini, Antigravity, Kimi, and other unsupported paths become `reconciliation_required`; partial or zero usage is never guessed. Journal payloads contain versioned billing/audit scalars and hashes, never prompts, response bodies, or credentials.

There is no Prometheus/OTel or public settlement endpoint. Existing structured logs emit backlog counts, oldest pending age, retry/failure totals, reconciliation totals, and provider-recovery totals once per minute. Operational procedures are in `docs/USAGE_SETTLEMENT_OPERATIONS.md`.

## Frontend / API contract

| Surface | Contract |
|---------|----------|
| Control plane (`/api/v1/auth`, `/user`, `/keys`, …) | `openapi/control-plane.yaml` + generated `frontend/src/api/generated/*` |
| Gateway (`/v1/*`, `/v1beta/*`, Codex, Antigravity, …) | Upstream protocol semantics. Not code-generated. |
| Public runtime config | `GET /api/v1/public/config` (canonical). `GET /api/v1/settings/public` is a compatibility alias. |
| Version skew | API: `GET /api/v1/public/version` → `{ version, min_frontend_version }`; frontend artifact: `sub2api-build.json` → `{ frontend_version, min_backend_version }` |

The SPA fetches public config at boot. Optional `window.__APP_CONFIG__` is only a first-paint cache (Vite dev inject). Embedded `index.html` is a static shell.

Frontend modules live under `frontend/src/features/` (API + some stores). Primitive UI is in `frontend/src/ui/`. Account platform fields go through `features/admin-accounts/form/registry.ts`. Settings uses `/admin/settings/:tab?`; most tab bodies are still in `SettingsView.vue`.

Build output remains `backend/internal/web/dist` so the compatibility `-tags embed` build can pack the same artifact. Frontend and backend versions are independent (`frontend/VERSION` and `backend/cmd/server/VERSION`). Release publishes an API image, a standalone frontend image/zip, and an embedded compatibility image.

## Deployment matrix

Separated same-origin deployment is the production default. Nginx owns the public port; the API is internal-only. Embedded delivery remains available for existing installations and rollback, but it is not the default production topology.

| Topology | How | Frontend | Public config | Upgrade | Notes |
|----------|-----|----------|---------------|---------|-------|
| **Separated compose** (default) | `docker-compose.yml` or `docker-compose.local.yml` | `sub2api-frontend` nginx image | `/api/v1/public/config` | Upgrade API or frontend independently | Host port is nginx; API is compose-network only. |
| **Separated host release** (production) | `component_deploy.py` + nginx/systemd/container hooks | Immutable release directory + atomic symlink | Same | Explicit `frontend`, `backend`, or `both` scope | One current and rollback pointer per component. |
| **Binary + embed** (compatibility) | `docker-compose.embedded.yml` or `-tags embed` | Served by the Go process | Same | Replace the paired artifact | Retained for existing operators and migration rollback. |
| **Split origin** (optional) | Static host + API process | Same zip on any static server | Same | Independent | Set `CORS_ALLOWED_ORIGINS` (or `cors.allowed_origins`) to the console origin. |

Reverse proxy requirements (official file: `deploy/nginx/sub2api.conf`):

- `underscores_in_headers on` (Codex / OpenAI `session_id`, `x-stainless-*`)
- WebSocket `Upgrade` / `Connection`
- `proxy_buffering off` for SSE / streamed gateway responses

Version skew is checked in both directions. At runtime the SPA warns if it is older than the API floor or if the API is older than the frontend floor. Deployment preflight fails closed on missing, malformed, or incompatible metadata. Cross-component contract changes use scope `both`.

## Related files

- OpenSpec change: `openspec/changes/refactor-architecture/`
- Control-plane spec: `openapi/README.md`
- Separated proxy: `deploy/nginx/README.md`
- Default compose: `deploy/docker-compose.yml`
- Embedded compatibility overlay: `deploy/docker-compose.embedded.yml`
- Component deployment: `deploy/component_deploy.py`
- Durable settlement operations: `docs/USAGE_SETTLEMENT_OPERATIONS.md`
