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

## Frontend / API contract

| Surface | Contract |
|---------|----------|
| Control plane (`/api/v1/auth`, `/user`, `/keys`, …) | `openapi/control-plane.yaml` + generated `frontend/src/api/generated/*` |
| Gateway (`/v1/*`, `/v1beta/*`, Codex, Antigravity, …) | Upstream protocol semantics. Not code-generated. |
| Public runtime config | `GET /api/v1/public/config` (canonical). `GET /api/v1/settings/public` is a compatibility alias. |
| Version skew | `GET /api/v1/public/version` → `{ version, min_frontend_version }` |

The SPA fetches public config at boot. Optional `window.__APP_CONFIG__` is only a first-paint cache (Vite dev inject). Embedded `index.html` is a static shell.

Frontend modules live under `frontend/src/features/` (API + some stores). Primitive UI is in `frontend/src/ui/`. Account platform fields go through `features/admin-accounts/form/registry.ts`. Settings uses `/admin/settings/:tab?`; most tab bodies are still in `SettingsView.vue`.

Build output is still `backend/internal/web/dist` so `-tags embed` can pack the same artifact. Release also publishes `sub2api-frontend-<version>.zip`.

## Deployment matrix

Default install, single-container compose, admin self-update, and the setup wizard are **unchanged**. Separated topology is opt-in.

| Topology | How | Frontend | Public config | Upgrade | Notes |
|----------|-----|----------|---------------|---------|-------|
| **Binary + embed** (default) | `deploy/install.sh` + systemd | Served by the Go process (`-tags embed`) | `/api/v1/public/config` | Admin self-update / replace binary | Same version as the binary. Setup wizard unchanged. |
| **Single-container compose** (default) | `docker-compose.yml` or `docker-compose.local.yml` | Embedded in `weishaw/sub2api` | Same | Pull image / recreate | Auto-setup. Preferred: `docker-compose.local.yml` via `docker-deploy.sh`. |
| **Separated compose** (optional) | Overlay `docker-compose.separated.yml` | nginx serves unpacked `sub2api-frontend-*.zip` | Same | Upgrade API image **and** frontend zip | Host port is nginx. API stays internal. See `deploy/nginx/README.md`. |
| **Split origin** (optional) | Static host + API process | Same zip on any static server | Same | Independent | Set `CORS_ALLOWED_ORIGINS` (or `cors.allowed_origins`) to the console origin. |

Reverse proxy requirements (official file: `deploy/nginx/sub2api.conf`):

- `underscores_in_headers on` (Codex / OpenAI `session_id`, `x-stainless-*`)
- WebSocket `Upgrade` / `Connection`
- `proxy_buffering off` for SSE / streamed gateway responses

Version skew: if the console build (`__APP_VERSION__` from `backend/cmd/server/VERSION`) is older than `min_frontend_version`, the SPA shows a dismissible banner. Embed matches by construction. A missing version endpoint does not block boot.

## Related files

- OpenSpec change: `openspec/changes/refactor-architecture/`
- Control-plane spec: `openapi/README.md`
- Separated proxy: `deploy/nginx/README.md`
- Compose overlay: `deploy/docker-compose.separated.yml`
