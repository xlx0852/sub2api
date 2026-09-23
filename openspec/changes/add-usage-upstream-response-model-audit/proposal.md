# Change: Audit the upstream response model in usage records

## Why

The usage table records the model the client asked for and the model the gateway sent upstream, but not
the model the upstream response actually declared. Operators therefore cannot see where a request
really landed, and a provider silently serving a different model (for example answering `gpt-6-sol`
for a `gpt-5.6-sol` request) leaves no trace in the record — which is exactly the mismatch the rollout
screenshots showed.

The gateway already extracts the response model into the forward result
(`ForwardResult.ResponseModel` / `OpenAIForwardResult.ResponseModel`, populated from the OpenAI
Responses stream terminal event, non-streaming bodies and the WS v2 relay), and an extraction helper
with tests already exists. Nothing persisted or displayed it.

## What Changes

- Persist two new columns on `usage_logs`: `upstream_response_model` (the model the upstream response
  declared, before any client-facing rewrite) and `upstream_model_mismatch` (tri-state: NULL when the
  response declared no model, otherwise whether the declared model differs from the model sent
  upstream).
- Extend the usage-log build path, repository insert/select plumbing, and the admin usage DTO so both
  fields round-trip.
- Show the response model under the model column with a `模型不一致 / Different model` badge whenever the
  response disagrees with the model sent upstream, on both the desktop table and the mobile card, and
  include both columns in the CSV export.
- Compare against the mapped model when a mapping was applied, falling back to the requested model, and
  treat model IDs case-insensitively.

## Impact

- Affected specs: `usage-route-audit` (new capability).
- Affected code: `backend/migrations/`, `backend/ent/schema/usage_log.go` (+ regenerated `backend/ent`),
  `backend/internal/domain/usage_log_entity.go`, `backend/internal/app/billing/`,
  `backend/internal/repository/usage_log_repo_{insert,query}.go`, `backend/internal/service/openai_gateway_usage.go`,
  `backend/internal/service/gateway_usage_billing.go`, `backend/internal/handler/dto/`,
  and the admin usage view/table, i18n and types on the frontend.
- The migration is additive and nullable, so applying it is instant and safe on a large table; no
  backfill is attempted and historical rows stay NULL.
- Existing rows read back with both fields unset, so no read path changes behaviour.
