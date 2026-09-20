## Context

The feature is implemented across the Ops advanced settings object, an in-memory stale-while-revalidate cache, account `extra` metadata, OpenAI scheduler policies, sticky selection checks, and admin forms. The screenshot covers the global 5h/7d controls; the backend search also identifies per-account overrides that must be removed to avoid a hidden remaining path.

## Decisions

- Remove only the OpenAI 5h/7d quota auto-pause path identified by `auto_pause_5h_threshold`, `auto_pause_7d_threshold`, `auto_pause_5h_disabled`, `auto_pause_7d_disabled`, and `openai_account_quota_auto_pause`.
- Leave `auto_pause_on_expired` and other provider-specific quota/rate-limit behavior unchanged.
- Historical JSON keys are tolerated as ignored data; no migration is needed because the keys are not schema columns.
- Add regression coverage proving an account over a historical threshold remains schedulable through the OpenAI selector.

## Validation

- Run focused scheduler, settings, account API, and frontend form tests before removal.
- Run backend tests, vet, staticcheck, frontend lint, typecheck, and Vitest after removal.
