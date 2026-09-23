# Change: Remove Anthropic OAuth/SetupToken account quota control

## Why

Production Anthropic supply is 100% APIKey: three accounts (`deepseek`, `kimi`, `国模`), all `type=apikey`, with empty `extra` and `session_window_*` columns `NULL`. `account_quota_windows` and `account_quota_usage_observations` contain zero Anthropic rows.

Every account-level quota-control gate is gated by `IsAnthropicOAuthOrSetupToken()` and returns "schedule anyway" for APIKey accounts. The stack is therefore inert in production while still occupying the account admin form, the scheduler policy chain, Redis caches, config keys, background wiring and telemetry. The earlier `retire-obsolete-gateway-services` change already removed the Claude OAuth CLI mimicry for the same reason; this change removes the remaining dead quota-control plane.

## What Changes

- Remove the 5h window-cost scheduler gate (`window_cost_limit`, `window_cost_sticky_reserve`), its prefetch context, cache reads/writes, telemetry and admin list enrichment.
- Remove the session-count gate (`max_sessions`, `session_idle_timeout_minutes`), `SessionLimitCache`, registration during selection and admin list enrichment.
- Remove the account RPM gate (`base_rpm`, `rpm_strategy`, `rpm_sticky_buffer`), `RPMCache`, post-forward increment and admin list enrichment.
- Remove the Anthropic-only user-message queue (`user_msg_queue_*`), its Redis cache, service, handler helper and global `gateway.user_message_queue` configuration.
- Drop the corresponding account DTO fields, admin form sections (window cost / session limit / RPM and the redundant Anthropic APIKey quota branch), capacity badges and localization keys.
- Keep TLS fingerprint, session-ID masking, cache-TTL override, custom base-URL relay, model routing, API-key passthrough and the generic API-key `quota_*` limits unchanged.

## Impact

- Affected specs: `account-selection` (platform constraints no longer include RPM/window-cost/session-count gates)
- Affected code:
  - `backend/internal/domain/account_entity.go`
  - `backend/internal/service/gateway_scheduling.go`, `gateway_scheduling_policy.go`, `gateway_service.go`, `gateway_runtime_stats.go`, `group_capacity_service.go`
  - `backend/internal/handler/gateway_handler.go`, `handler/admin/account_handler.go`, `handler/dto/*`
  - `backend/internal/repository/{rpm_cache,session_limit_cache,user_msg_queue_cache,scheduler_cache,wire}.go`
  - `backend/internal/config/*`
  - `frontend/src/components/account/{AccountCapacityCell,CreateAccountModal,EditAccountModal}.vue`, account types and i18n
- No protocol, billing, quota-ledger or deployment contract changes.
- Stored historical `extra` keys remain inert; no destructive data migration is performed.
- Rollback: restore the deleted files and re-add the removed gates; no schema change is involved.
