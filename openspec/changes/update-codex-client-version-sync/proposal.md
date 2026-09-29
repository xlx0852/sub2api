# Change: Wire the Codex client version auto-sync runner into the application

## Why

The Codex client version auto-sync feature does not run at all in production. It is not a network,
rate-limit, or GitHub problem: the service is never constructed by the compiled injector.

Evidence (all read-only, gathered 2026-09-30 against the running deployment):

- `OpenAICodexVersionSyncService` exists, is exported through
  `ProvideOpenAICodexVersionSyncService` (which calls `svc.Start()`), and is listed in the service
  `ProviderSet` (`backend/internal/service/wire.go`).
- `backend/cmd/server/wire_gen.go` — the file that actually compiles; `cmd/server/wire.go` is
  excluded by its `wireinject` build tag — contains **zero references** to
  `ProvideOpenAICodexVersionSyncService`, and so does the injector of the deployed revision
  (`vcs.revision = 2694ea76c2fbbd0b791d0bf10a9feae3e74ffe4d`, `version 0.1.392`,
  `grep -c CodexVersionSync wire_gen.go` = 0 for both).
- Production settings rows (`settings` table, read-only):

  | key | value | updated_at |
  | --- | --- | --- |
  | `openai_codex_client_version` | (empty) | 2026-09-21 14:45:40+08 |
  | `openai_codex_client_version_synced` | `0.153.4` | **2026-09-05 07:06:15+08** |
  | `openai_codex_version_auto_sync_enabled` | `true` | 2026-09-21 14:45:40+08 |

- The synced value has not changed for 3.5 weeks while upstream released `rust-v0.159.1`
  (published 2026-09-29T20:32:34Z), the toggle is enabled, and the running container logs contain
  **zero** lines matching `openai_codex_version_sync` — the 6-hourly runner is not executing.
- The container's egress to `https://api.github.com` returns `200`, and no proxy or
  `UPDATE_GITHUB_TOKEN` is configured, so nothing outside the code prevents the sync from working.

Wire semantics explain the failure: a provider that nothing in the injector graph consumes is
silently dropped from the generated graph. `wire check ./cmd/server` reports no diagnostics while
`wire diff ./cmd/server` shows the generated graph — and neither mentions the version sync service.
The service therefore never starts, and there is no build-time or runtime signal that the feature is
inert. The same class of silent omission also affects `repository.ProvideGitHubReleaseClient`
(`internal/repository/wire.go:51`), which has no consumer either.

Operator-visible consequence: the effective Codex client version
(`GetOpenAICodexClientVersion`: panel value → synced value → compiled constant) is pinned to the last
hand-written value. Because the Codex models manifest is client-version-gated — the same request
returned 5 models at `0.153.4` and 8 models at `0.159.x` (see
`openspec/changes/add-oauth-upstream-model-sync/design.md`) — the advertised entitlement set stays
stale, OAuth upstream model sync under-reports available models, and outbound Codex identity claims
an outdated version. Keeping up with the upstream client requires an administrator to notice the
freeze and edit the panel field by hand, which is exactly what the automatic sync was built to avoid.

## What Changes

- **Make the runner a compiler-enforced dependency**: the shutdown assembly (`provideCleanup` in
  `cmd/server/cleanup.go`) takes the sync service as a **required parameter** and registers its
  `Stop` with the lifecycle registry. Because the injector must satisfy that parameter to build, a
  provider that only exists in `ProviderSet` can no longer be silently unreachable, and the runner's
  goroutine is stopped on shutdown.
- **Regenerate `cmd/server/wire_gen.go`** with the pinned `github.com/google/wire v0.7.0` instead of
  hand-adding a call. Today's regeneration is a pure variable-rename diff (156 added / 156 removed
  lines, `go build ./...` passes) that proves the checked-in file drifted from the graph; after the
  change `wire diff ./cmd/server` must report nothing.
- **Make the sync observable**: log the runner start (with interval), each skip with its reason
  (already fresh / disabled / version not advanced), and keep the existing advance and failure logs.
  A frozen value must be distinguishable in logs from "upstream has released nothing new".
- **Preserve every existing sync semantic**: 6h interval, startup skip when the value is fresher than
  one interval, forward-only advancement, failure retention (never clear or downgrade), default-on
  toggle, `rust-v` tag-prefix filtering, and panel-overrides-synced precedence.

Non-goals: no change to the interval, the fetched repository, the version resolution order, or the
admin API/DTO shape; no new setting; no frontend change; no GitHub token or proxy is introduced; the
OpenAI status collector and unrelated wiring are untouched.

## Impact

- Affected specs: `codex-client-version-sync` (new capability; no existing spec covers the client
  version setting or its sync runner).
- Affected code: `backend/cmd/server/cleanup.go` (dependency + stop step),
  `backend/cmd/server/wire_gen.go` (regenerated), `backend/cmd/server/wire_gen_test.go` (dependency
  list + a test that shutdown stops the runner),
  `backend/internal/service/openai_codex_version_sync_service.go` (+ its test) for the skip logs,
  and `backend/go.sum` for the `github.com/google/subcommands` hash that `wire`'s CLI imports.
- No database migration and no API change. Existing setting rows are kept; on the first start after
  deployment with a stale or missing synced value, `runInitial` writes the current latest stable
  version (today `rust-v0.159.1` → normalized `0.159.1`) and invalidates the cached effective
  version.
- Expected operator-visible result after deployment: `openai_codex_client_version_synced` moves off
  `0.153.4` within one startup, a `openai_codex_version_synced` log line records the transition, and
  the Codex models manifest for OAuth accounts grows from 5 to 8 visible models. Operators who need a
  pinned version set the panel field, which the sync never overwrites.
