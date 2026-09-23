# Change: One diagnostics plan per account, a next-run countdown, and removal of model purity attribution

## Why

Three things came out of running diagnostics in production:

1. **Diagnostics settings had no single owner.** An account could accumulate several scheduled test plans
   (`scheduled_test_plans` had no uniqueness constraint), each with its own cron, model and auto-recover flag. The
   "add plan" button invited even more, and nobody could say which plan represented the account's diagnostics.
2. **The next execution time was frozen text.** Showing "next run in 3 minutes" only recomputed on re-render, so an
   operator watching the panel could not tell how long was left or whether a run had just fired.
3. **Model purity attribution did not earn its cost.** It sent several hundred integers per challenge to the
   account's upstream, which bills the account's own quota while being invisible in our usage ledger, and the result
   was only a closed-set probability — in one production case it pointed at a different model than the account
   claimed, but the operator had no action to take and stopped using it. The operator asked for it to be removed
   completely.

## What Changes

- Enforce **exactly one** scheduled test plan per account: a second create is rejected with a conflict error, a
  unique index backs the rule, and the UI hides the create entry once a plan exists. Existing duplicates are
  collapsed to the newest enabled plan by migration (production had none).
- Show the next execution as a **live countdown** in both diagnostics views (the plans panel and the account usage
  drawer), with an automatic single reload when a deadline passes.
- **Remove model purity attribution end to end**: the Go package and parity fixtures, the probe service, API
  endpoints, domain/ports/repository code, the diagnostics integration (plan settings, result linkage and the
  challenge-as-probe path), the fingerprint bank resources and release wiring, the admin UI (menu entry, list badge,
  panel controls, result rendering), the i18n keys, the config keys, and finally the database objects
  (`model_attribution_runs`, `scheduled_test_results.purity_run_id`, the three purity columns on
  `scheduled_test_plans`). Diagnostics returns to the lightweight connectivity chain, which is the only probe again.

## Impact

- Affected specs: `account-diagnostics` (new capability).
- Affected code: migrations 209/210, plan/result repositories and DTOs, `ScheduledTestService`,
  `ScheduledTestRunnerService`, the diagnostics panel, the usage drawer and the account action menu, plus a static
  repository test that keeps plan row projections in sync with the scanner.
- Removed data: stored attribution runs were exported to
  `/opt/sub2api/backups/model_attribution_runs-<ts>.sql` on the production host before the table was dropped.
