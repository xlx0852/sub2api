## 1. Single plan per account

- [x] 1.1 Migration `209_single_scheduled_test_plan_per_account.sql`: collapse duplicates (enabled → newest → highest
  id wins) and add `uniq_scheduled_test_plans_account` on `scheduled_test_plans(account_id)`.
- [x] 1.2 `ScheduledTestService.CreatePlan` rejects a second plan with `ErrScheduledPlanExists`; the handler maps it
  to HTTP 409.
- [x] 1.3 The diagnostics panel hides the create entry once a plan exists (zh/en copy).

## 2. Next-execution countdown

- [x] 2.1 `useExecutionCountdown` composable: one ticking clock per view, `分/秒` (hours for longer intervals)
  formatting, "正在执行…" at zero, one reload per distinct deadline, and no timer left behind on close/unmount.
- [x] 2.2 Used by both diagnostics views: the plans panel (embedded and dialog headers) and the account usage
  drawer's plan header.
- [x] 2.3 Tests: `ScheduledTestsPanel.countdown.spec.ts` (6 cases) plus drawer coverage. The suite caught a real
  timer leak — the open path started the clock after an in-flight load, so a panel closed during loading kept a
  countdown ticking in the background forever.

## 3. Remove model purity attribution (complete)

- [x] 3.1 Deleted the Go implementation: `internal/pkg/modelattribution/**` (including the bank and the
  Python-parity fixtures), `service/model_attribution_service*.go`, `handler/admin/account_model_purity_handler.go`,
  `domain/model_attribution.go`, `ports/model_attribution_port.go`, `repository/model_attribution_repo.go`.
- [x] 3.2 Removed the diagnostics integration: plan settings (`auto_purity`, interval, answers), result link
  (`purity_run_id`, enriched `purity` payload), the challenge-as-probe path and its failure classification, the
  feature switch, and the service/repository/Wire/config wiring.
- [x] 3.3 Removed the admin surface: account action-menu entry, account-list badges, panel controls and badges,
  drawer result rendering, the purity modal component and specs, the API client functions and types, and every zh/en
  i18n key.
- [x] 3.4 Removed the release plumbing: `backend/resources/model-attribution/**`, the `package-release.py` entry and
  its validation block, the GoReleaser archive entry, the `deploy/config.example.yaml` block and the AGENTS.md note.
- [x] 3.5 Migration `210_drop_model_purity_attribution.sql` drops `scheduled_test_results.purity_run_id` **before**
  dropping `model_attribution_runs` (the foreign key dependency) and then the three plan columns. Attribution data was
  exported to `/opt/sub2api/backups/model_attribution_runs-20260923043047.sql` before the drop.
- [x] 3.6 Kept the single-plan rule and the countdown, which are independent of purity.

## 3b. Reasoning probe replaces the greeting

- [x] 3b.1 `diagnostic_probe.go`: the sweets problem (predictable integer answer), the expected value kept server
  side, a tolerant answer parser (marker forms, fenced blocks, "reasoning then answer", plus refusal detection) and
  the one-line verdict.
- [x] 3b.2 Every text test path sends the problem: Claude messages, OpenAI chat completions, OpenAI responses
  (a dedicated diagnostic payload builder so the 15s usage/quota probe keeps its trivial prompt) and Gemini text.
  Image-generation tests keep their image prompts.
- [x] 3b.3 The verdict is appended once, at the SSE exit of `TestAccountConnection`, so both the interactive test
  and the diagnostics history carry it. An earlier attempt also annotated inside the background runner, which
  produced the verdict twice in production; the runner no longer annotates and a test asserts the source stays that
  way.
- [x] 3b.3b Live runs exposed two costs of the probe and both are handled: latency rose from ~4s to 19-36s, and a
  slow upstream made diagnostics results fail (`context deadline exceeded`), which would have fed the quota-wall
  path and dropped the availability rate. Partial content plus a timeout error is now recorded as connected with an
  explicit "本样本未判定" note, and the per-model diagnostics timeout went from 90s to 120s (still inside the 4-minute
  batch budget). The prompt asks for a brief answer to keep latency down.
- [x] 3b.3c The answer key is configurable (`diagnostics.reasoning_probe_expected_answer`, default the strictly
  derived 29) and the report always names the key it used; when a non-default key is set and the model produced the
  strictly derived answer, the report discloses that as well. Production was set to the operator's key (21) on
  request, and the live probe then reads:
  `[降智检测] 期望 21，模型给出 29 → 不正确（疑似降智，建议多次复测确认）（注：按题面严格推导应为 29，当前判定键为 21）`.
- [x] 3b.4 Tests: prompt contains neither the right answer nor the popular wrong one; the expected value is pinned to
  the verified one; twelve answer shapes; the SSE verdict is emitted for text and skipped for image/error replies;
  the verdict appears exactly once in a recorded result.

## 4. Verification

- [x] 4.1 `go build ./...` clean; `go vet` and `go test` pass for `internal/handler/...`, `internal/repository/`,
  `internal/config/`, `internal/ports/`, `internal/domain/`; the only `internal/service` failures are the six
  pre-existing ones documented earlier.
- [x] 4.2 Frontend: `vue-tsc --noEmit` clean; component/i18n/view suites pass apart from the two pre-existing
  `UserPlatformQuotaModal` cases and the pre-existing `AccountsView.sparkShadow` case.
- [x] 4.3 Live verification on the production host:
  - migration 210 applied: `model_attribution_runs` absent and zero `%purity%` columns remain;
  - `GET /api/v1/admin/accounts/126/purity-test` and `POST /api/v1/admin/accounts/purity-tests/batch` both return
    **404**;
  - diagnostics ran again through the connectivity chain (results `178443`/`178441`/`178428`, `Hi! How can I help…`,
    no purity link);
  - served entry chunk is byte-identical to the local build and the served panel chunk contains **no** purity
    strings while still containing the countdown and single-plan copy;
  - the leftover candidate container `…-routepair3-…` (old binary, expects the dropped columns, not in the nginx
    upstream) was stopped so it cannot run diagnostics against a schema it does not understand.
- [x] 4.4a Live check of the reasoning probe (account 126, model `swe-2`): the interactive test answered **21** and
  was flagged `不正确`; two diagnostics runs answered **29** and were flagged `正确`. The upstream is therefore
  *sometimes* correct, which is why the report wording asks for repeat samples, and why the diagnostics history —
  not a single click — is the signal to watch. Diagnostics latency rose from ~4s ("hi") to 19–34s (reasoning).
- [x] 4.4 A first attempt at migration 210 dropped the table before the referencing column, which crash-looped the
  live container; the previous binary was rolled back within a minute to restore service, the migration was
  corrected, and it then applied cleanly. The lesson (drop referencing columns before referenced tables) is recorded
  in the migration comment.
- [x] 4.5 `openspec validate enforce-single-diagnostics-plan --strict`.

## Deployment record (2026-09-23)

- Backend `0.1.391-nopurity2-8e5416807` hot-swapped into the live container's release dir; rollback artifacts
  `sub2api.bak-nopurity-20260923043047`, `sub2api.bak-nopurity2-20260923043234`.
- Frontend release `0.1.391-nopurity-072bf6594`; `frontend/current` repointed, `frontend/rollback` →
  `0.1.391-drawer-9c4f2a7b1`.
- The obsolete `add-model-purity-attribution` change directory was deleted (never applied to `specs/`), and this
  change now carries the removal.
