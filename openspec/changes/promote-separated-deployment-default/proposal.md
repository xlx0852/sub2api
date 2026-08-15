# Change: Promote separated frontend/API deployment to the default

## Why

The frontend is already environment-agnostic and an official separated compose/nginx topology exists, but production guidance still treats the embedded binary as the default and deploys the API and console as one rollback unit. This coupling caused a frontend rendering problem to require an API deployment even though the backend was healthy.

## What Changes

- **BREAKING (deployment default):** make the separated same-origin topology the default production layout; keep embedded packaging as an explicit compatibility option.
- Give the frontend and backend independent version sources, artifacts, current/rollback pointers, health gates, and rollback procedures.
- Add an explicit deployment scope (`frontend`, `backend`, or `both`). A frontend-only change SHALL NOT restart or replace the API, and a backend-only change SHALL NOT rebuild or replace the frontend.
- Add two-way compatibility preflight. A contract-incompatible change is classified as `both` and is deployed in a compatibility-safe order instead of pretending it is a single-side release.
- Rewrite the production blue-green guide, compose/nginx guidance, architecture matrix, and upgrade/rollback checklist around component-scoped releases.
- Provide a migration procedure from the current embedded production topology without changing protocol I/O, scheduling, billing, configuration keys, database schema, or public URLs.

## Impact

- Affected spec: `frontend-backend-separation`
- Affected deployment surfaces: `deploy/docker-compose*.yml`, `deploy/nginx/`, `deploy/SAFE_BLUE_GREEN_DEPLOY_CN.md`, `deploy/README.md`, `deploy/install.sh`/release automation as applicable
- Affected version/build surfaces: `frontend/vite.config.ts`, frontend startup compatibility check, backend public version response, `.github/workflows/release.yml`, root/deploy Makefiles
- Affected architecture docs: `docs/ARCHITECTURE.md`, `docs/ARCHITECTURE_CN.md`
- Production migration: static frontend release directory + atomic symlink, API-only container ports, Nginx same-origin routing, independent frontend/API rollback markers

## Non-Goals

- No microservice split of backend business modules.
- No change to gateway protocol semantics, authentication, scheduling, billing, YAML/env key names, database contents, or public domain/path layout.
- No removal of `-tags embed`; it remains a supported compatibility distribution.
- No automatic inference from the whole dirty worktree as release authority; the deploy scope and file manifest remain explicit.
