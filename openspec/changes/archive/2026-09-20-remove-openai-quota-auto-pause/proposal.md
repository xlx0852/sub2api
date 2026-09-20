# Change: Remove OpenAI 5h/7d quota auto-pause

## Why

The OpenAI account 5-hour and 7-day usage-threshold auto-pause feature adds global settings, per-account metadata, scheduler policy branches, cache refresh logic, and UI controls. The requested behavior is to remove these controls and the corresponding enforcement path so quota usage no longer silently excludes accounts during scheduling.

## What Changes

- Remove the global Ops settings for 5h/7d quota auto-pause.
- Remove per-account 5h/7d threshold and disable-flag fields from account forms and payloads.
- Remove OpenAI quota auto-pause scheduler policy, cache, context plumbing, and related tests.
- Preserve unrelated expiry auto-pause, provider rate-limit cooldown, and Grok quota handling.
- Keep unknown historical `extra` keys inert and do not perform destructive data rewrites.

## Impact

- Affected code: OpenAI scheduling, settings API, account admin API/forms, localization, and tests.
- Affected capability: account scheduling and admin configuration.
- This is a behavior and control-plane removal; existing accounts will no longer be skipped solely for 5h/7d OpenAI usage thresholds.
