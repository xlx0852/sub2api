# Tasks: Wire the Codex client version auto-sync runner into the application

## 1. Assembly

- [x] 1.1 Add a `*service.OpenAICodexVersionSyncService` parameter to `provideCleanup`
      (`backend/cmd/server/cleanup.go`) and register a stop step that calls `Stop()` behind a nil
      guard, next to the other background services.
- [x] 1.2 Regenerate `backend/cmd/server/wire_gen.go` with
      `GOFLAGS=-mod=mod go run github.com/google/wire/cmd/wire gen ./cmd/server` (wire v0.7.0, the
      version pinned in `go.mod`) so the file matches the provider graph; keep the generated header.
- [x] 1.3 Confirm the generated graph constructs the runner: `grep -c CodexVersionSync
      cmd/server/wire_gen.go` is non-zero and the runner is passed to `provideCleanup`.
- [x] 1.4 Keep `backend/go.sum` in a state where the generator runs offline
      (`github.com/google/subcommands`, imported by `wire`'s CLI). Do not add anything else.
- [x] 1.5 Verify `go build ./...` and that `wire diff ./cmd/server` reports no difference.

## 2. Observability

- [x] 2.1 Log a start event (including the interval) from `Start()` after the dependency guards, so a
      runner that never starts is visible.
- [x] 2.2 Log the initial-sync skip with the age of the stored value.
- [x] 2.3 Log no-op runs with the reason (`disabled`, `up_to_date`, `not_advanced`); keep the
      existing advance, persistence-failure, and fetch-failure event names.

## 3. Tests

- [x] 3.1 Extend `TestProvideCleanup_WithMinimalDependencies_NoPanic` in
      `backend/cmd/server/wire_gen_test.go` with the new dependency (explicitly `nil`, matching the
      file's existing style) and confirm it still passes.
- [x] 3.2 Add a test that a started runner is stopped: the goroutine exits and `Stop()` is
      idempotent (`internal/service`, where the setting/GitHub stubs live), plus cleanup on a nil
      runner does not panic (`cmd/server`).
- [x] 3.3 Keep the existing service tests green: interval/priority resolution, forward-only
      advancement, disabled toggle, fetch failure retention, `rust-v` filtering, list fallback, and
      start-requires-dependencies.
- [x] 3.4 Add a test asserting the no-op reason is reported for "disabled", "up to date", and
      "not advanced", by capturing the log output.
- [x] 3.5 Assert no test logs credential material or upstream response bodies.

## 4. Verification

- [x] 4.1 `go build ./...`, `gofmt -l` clean, `go test ./internal/service/ ./cmd/server/ -count=1`.
      Six tests in `internal/service` fail on this revision; all six fail identically on the
      pre-change tree (verified by stashing the change and re-running), so they are unrelated:
      `TestIsModelRateLimited`, `TestGetModelRateLimitRemainingTime`,
      `TestGetRateLimitRemainingTime`,
      `TestOpenAIGatewayServiceForward_CodexImageInjectionRespectsGroupCapability`,
      `TestOpenAIGatewayService_OAuthPassthrough_CompactUsesJSONAndKeepsNonStreaming`,
      `TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexImageBridgeRespectsResponsesLite`.
- [ ] 4.2 Confirm the runner is present in a real binary (build with the `cmd/server` package and
      check symbols), not just in the source; guard the generated injector with a source-level test
      (`cmd/server/wire_gen_test.go`) so a future regeneration that drops the runner fails the
      build's test step rather than silently stopping the sync.
- [x] 4.3 `openspec validate update-codex-client-version-sync --strict` passes.

## 5. Documentation

- [x] 5.1 Record the root cause, the wire semantics, and the rejected alternatives in `design.md`.
- [x] 5.2 Note in `proposal.md` (Impact) that the first start after deployment advances the synced
      version and invalidates the cached effective version, and that the panel field still overrides
      the synced value.
