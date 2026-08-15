## Context

Sub2API currently has all technical pieces for separated deployment: a static SPA artifact, runtime config APIs, a version endpoint, an nginx reference, and a compose overlay. Operationally, however, the default production runbook builds `-tags embed`, switches one API container, and treats the frontend and backend as a pair. The shared `backend/cmd/server/VERSION` also makes a frontend-only release look like a backend release.

## Goals / Non-Goals

### Goals

- Make same-origin nginx + static frontend + internal API the default production topology.
- Allow frontend-only and backend-only releases without touching the unaffected component.
- Preserve artifact-only builds, blue-green validation, one retained rollback per component, and the existing public origin.
- Fail closed when a proposed single-side release is incompatible with the deployed counterpart.

### Non-Goals

- Splitting the Go modular monolith into services.
- Changing API/gateway behavior or configuration keys.
- Removing embedded distributions for existing operators.
- Performing production migration before staging and explicit deployment approval.

## Decisions

### Decision: separated same-origin is the production default

Nginx owns the public listener. `/` and SPA fallbacks are served from an atomic frontend symlink; control-plane, gateway, SSE, and WS paths proxy to the selected API loopback port. The API binary is built without `-tags embed` for separated production.

Embedded packaging moves from “default” to “compatibility topology”. Existing installations may stay embedded until explicitly migrated.

### Decision: explicit component scope is release authority

The deployment entry point accepts exactly one scope:

- `frontend`: build/upload/switch only the frontend static artifact.
- `backend`: build/upload/switch only the API binary and runtime resources.
- `both`: deploy both components in a compatibility-safe order.

Scope may be suggested from an explicit file manifest, but it MUST NOT be inferred from every dirty-worktree path. OpenAPI/generated-client, runtime-config schema, authentication contract, or version-compatibility changes are classified as `both` unless the compatibility tests prove a single-side rollout is safe.

### Decision: independent versions and compatibility floors

- Backend version source remains `backend/cmd/server/VERSION`.
- Frontend gains `frontend/VERSION`; Vite no longer reads the backend version file.
- The backend public version response continues to expose `version` and `min_frontend_version`.
- The frontend artifact embeds `frontend_version` and `min_backend_version` build metadata.
- A frontend-only switch is rejected when the live backend is below the artifact's minimum backend version.
- A backend-only switch is rejected when the live frontend is below the candidate backend's minimum frontend version.

Compatibility metadata is release metadata, not deployment-specific configuration, so the SPA artifact remains environment-agnostic.

### Decision: independent blue-green and rollback state

API releases retain the current alternate-port/container flow. Frontend releases use immutable directories and an atomic symlink:

```text
/opt/sub2api/frontend/releases/<version>-<build>/
/opt/sub2api/frontend/current  -> releases/<new>
/opt/sub2api/frontend/rollback -> releases/<old>
```

Markers are component-scoped (`.codex-api-current-*`, `.codex-api-rollback-*`, `.codex-frontend-current`, `.codex-frontend-rollback`). A frontend rollback never restarts the API; an API rollback never changes the frontend symlink.

For `both`, deploy the backward-compatible API first, verify the existing frontend, then switch the frontend. Rollback reverses only the failing component unless the compatibility contract requires the pair.

### Decision: component-specific gates

Frontend-only gates:

- artifact SHA and immutable file inventory
- `index.html`, hashed assets, SPA fallback, public config/version fetch
- login/settings route render smoke test
- compatibility preflight against the live API

Backend-only gates:

- targeted Go tests and non-embed Linux build
- loopback `/health`, public config/version, auth 401, SSE/WS/gateway checks relevant to the change
- compatibility preflight against the live frontend
- settlement/usage invariants when the change affects data paths

Database backup is mandatory only for backend releases that can migrate or mutate persisted state. Frontend-only releases do not dump PostgreSQL or touch API containers.

## Risks / Trade-offs

- Independent releases can create version skew. Mitigation: two-way compatibility floors and fail-closed preflight.
- Default compose migration changes operator expectations. Mitigation: retain and document an embedded compatibility file and an explicit migration/rollback path.
- Nginx route omissions can break SSE/WS or expose the API port. Mitigation: keep one authoritative proxy include, `nginx -t`, route-contract tests, and loopback-only API binding.
- Two rollback histories add operational state. Mitigation: exactly one current and one rollback artifact per component, with component-scoped cleanup.
- Frontend cache can preserve an old SPA across switches. Mitigation: no-cache `index.html`, immutable hashed assets, version compatibility warning, cache-busted origin validation.

## Migration Plan

1. Add independent version/compatibility metadata and tests while current embed remains live.
2. Make compose/nginx and deployment tooling support component scopes in staging.
3. Build a non-embed API candidate and static frontend candidate from explicit manifests.
4. In staging, prove frontend-only, backend-only, both, and independent rollback paths, including SSE/WS and login/settings rendering.
5. On production, back up current nginx/API metadata and database as required, stage the static frontend, start the non-embed API beside the embedded container, and switch nginx atomically.
6. Retain the embedded container/config as migration rollback until the observation window passes; then establish independent frontend/API rollback pointers.

## Open Questions

- Whether upstream/community `install.sh` should switch defaults immediately or first emit a migration prompt for existing installations. The production runbook and official compose default will still become separated under this change.
