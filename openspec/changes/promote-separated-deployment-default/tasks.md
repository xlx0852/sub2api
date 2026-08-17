## 1. Version and compatibility contract

- [x] 1.1 Add `frontend/VERSION` and make Vite/release packaging use it independently from the backend version.
  - 2026-08-15: Added independent frontend/backend version and compatibility-floor files; Vite, build metadata, and release packaging now read the frontend version independently. Verified frontend typecheck/build and targeted backend version-handler tests.
- [x] 1.2 Add frontend `min_backend_version` build metadata and two-way semver compatibility checks with fail-closed deployment preflight.
  - 2026-08-16: Frontend build metadata now carries `frontend_version`/`min_backend_version`; API carries its independent frontend floor. Runtime warnings and manifest preflight check both directions and reject missing, malformed, or incompatible metadata.
- [x] 1.3 Add tests for compatible frontend-only, compatible backend-only, and incompatible single-side releases.
  - 2026-08-16: Added compatible/incompatible frontend-only and backend-only cases, contract-change scope rejection, and compatibility-ordered `both` coverage in `deploy/tests/test_component_deploy.py`.

## 2. Default separated topology

- [x] 2.1 Make the official production compose layout nginx static frontend + internal API by default; retain an explicit embedded compatibility topology.
  - 2026-08-16: Both official Compose files now publish only `sub2api-web`; API is internal. Added `docker-compose.embedded.yml`, standalone frontend image packaging, and distinct API/frontend/embedded release images.
- [x] 2.2 Keep the API loopback/internal only and verify nginx routing for control plane, gateway, SSE, WS, underscore headers, and SPA fallback.
  - 2026-08-16: Added route-contract tests for all public protocol families, SSE buffering, WS upgrade, underscore headers, internal upstream, and SPA fallback; later verified the config with a real nginx container and Compose v2 containerized rendering in 5.1.
- [x] 2.3 Add immutable frontend release directories and atomic `current`/`rollback` symlinks.
  - 2026-08-16: `component_deploy.py` installs SHA-addressed immutable releases and atomically replaces relative current/rollback symlinks.
- [x] 2.4 Add component-scoped API and frontend current/rollback markers and cleanup rules.
  - 2026-08-16: Added independent `.codex-api-*`/`.codex-frontend-*` markers and retention limited to each component's current plus rollback releases.

## 3. Component-scoped deployment workflow

- [x] 3.1 Add an explicit `frontend|backend|both` deployment entry point using an explicit artifact/file manifest.
  - 2026-08-16: Added `deploy/component_deploy.py` and example JSON manifest. Artifact transfer is an explicit per-file step; SHA-256 is verified before install.
- [x] 3.2 Implement frontend-only build, upload, verification, atomic switch, and rollback without restarting or replacing the API.
  - 2026-08-16: Added frontend artifact build/upload procedure, metadata/file validation, atomic switch/rollback, and a regression asserting API marker/DB state remain unchanged.
- [x] 3.3 Implement backend-only non-embed build, upload, alternate-port verification, switch, and rollback without rebuilding or replacing frontend files.
  - 2026-08-16: Added non-embed artifact procedure plus mandatory explicit alternate-port verify and switch hooks; unit coverage proves frontend state is unchanged.
- [x] 3.4 Implement `both` ordering and rollback, forcing coupled deployment only when compatibility/contract checks require it.
  - 2026-08-16: `both` verifies backward-compatible API transition first, switches API, then frontend. `contract_change=true` rejects single-component scope.
- [x] 3.5 Scope PostgreSQL backup to backend releases that may migrate or mutate persisted state; frontend-only releases must not touch DB/API runtime state.
  - 2026-08-16: DB backup hook is accepted only on backend paths and is mandatory only when the manifest declares `db_change=true`.

## 4. Documentation

- [x] 4.1 Rewrite `deploy/SAFE_BLUE_GREEN_DEPLOY_CN.md` around component classification, independent versions, gates, markers, and rollback.
  - 2026-08-16: Replaced the paired embed runbook with component scopes, manifests, independent state, gates, and rollback commands.
- [x] 4.2 Update `docs/ARCHITECTURE.md`, `docs/ARCHITECTURE_CN.md`, `deploy/README.md`, and `deploy/nginx/README.md` so separated is default and embed is compatibility-only.
  - 2026-08-16: Updated English/Chinese architecture and deployment guidance; old separated overlay is retained only for command compatibility.
- [x] 4.3 Document migration from the current embedded production topology and the exact rollback to the retained embedded container/config.
  - 2026-08-16: Added migration sequence, observation-window retention, and exact embedded Compose rollback command.
- [x] 4.4 Document examples for frontend-only UI fixes, backend-only scheduler/billing fixes, and coupled OpenAPI/runtime-config changes.
  - 2026-08-16: Runbook classifies UI, scheduler/billing, and cross-component contract changes and documents each scope's commands and invariants.

## 5. Verification and rollout

- [x] 5.1 Validate compose rendering, nginx configuration, shell scripts, frontend build/typecheck/tests, backend non-embed build/tests, `git diff --check`, and OpenSpec strict.
  - 2026-08-16: Root `make build`, targeted version/route Go tests, embed compatibility compile, frontend semver tests (7), deploy/nginx tests (11), real `nginx:1.27-alpine nginx -t`, Compose v2 default/embedded topology assertions, YAML parse, Python compile, `git diff --check`, and strict OpenSpec validation passed. Existing Vite chunk warnings remain non-fatal.
- [ ] 5.2 In staging, execute frontend-only, backend-only, and both deployments; verify login/settings, public config/version, HTTP/SSE/WS, and independent rollbacks.
- [ ] 5.3 Migrate production only after staging evidence and explicit approval; retain the embedded deployment as rollback through the observation window.
  - 2026-08-16: Explicitly approved backend-only production rollout of API `0.1.322` completed on alternate port 18247; frontend `0.1.321` symlink/marker was unchanged. API `0.1.321` on 18246 is retained as the independent rollback container/release. This does not satisfy or waive the missing staging matrix in 5.2, so 5.3 remains unchecked.
- [ ] 5.4 After production verification, update component markers/retention and archive the change; do not claim completion earlier.
