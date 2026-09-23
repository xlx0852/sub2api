## 1. Storage

- [x] 1.1 Add the nullable `upstream_response_model` / `upstream_model_mismatch` columns to `usage_logs`
  via migration `206_add_usage_log_upstream_response_model.sql`.
- [x] 1.2 Add both fields to the ent schema and regenerate the ent package.

## 2. Write path

- [x] 2.1 Add `UpstreamResponseModel` to the provider-neutral `RecordInput` and map both fields in
  `BuildUsageLog`, deriving the tri-state mismatch from the sent model (mapped model, falling back to
  the requested one).
- [x] 2.2 Pass the observed response model from the OpenAI usage recording and the gateway billing
  usage recording.
- [x] 2.3 Thread the two columns through `prepareUsageLogInsert`, the arg-type table, all eight
  INSERT/CTE column lists, both placeholder blocks, and the repository select/scan path.

## 3. Read path and presentation

- [x] 3.1 Expose both fields on the admin usage DTO and mapper.
- [x] 3.2 Show the upstream response model under the model column with a mismatch badge (desktop table
  and mobile card) and add the two columns to the CSV export.
- [x] 3.3 Add `usage.upstreamResponseModel` / `usage.upstreamModelMismatch` / `usage.modelMismatch` to
  the zh and en locales and the frontend types.

## 4. Verification

- [x] 4.1 `backend/internal/app/billing` tests cover the mismatch derivation: differing model, equal
  model, case-insensitive equality, mapped-model comparison, and the tri-state NULL case.
- [x] 4.2 Update the repository arg-index assertions for the two additional bound parameters and keep
  the arg-count/type parity test passing.
- [x] 4.3 Frontend tests assert the annotation and badge render on mismatch and stay absent when the
  upstream declared no model.
- [x] 4.4 `go build ./...` clean; `go test ./...` shows no new failures beyond the six pre-existing
  `internal/service` failures; frontend `vue-tsc --noEmit` and the usage component suites pass.
- [x] 4.5 Run `openspec validate add-usage-upstream-response-model-audit --strict`.

## 5. Deployment

- [x] 5.1 Carry the two fields through `domain.UsageLogSnapshot` so the usage-settlement payload keeps
  them across the authoritative in-transaction insert (`NewUsageLogSnapshot` / `UsageLog()`). Add the
  reflection round-trip guard so a future snapshot omission fails a test instead of silently dropping
  a column.
- [x] 5.2 Capture the response model on every live OpenAI upstream path: the Responses JSON and
  SSE-to-JSON handlers, both chat-completions handlers, `forwardOpenAIWSV2`, the client-WS ingress
  relay, the HTTP-to-WS bridge, and the WS v2 passthrough relay/adapter.
- [x] 5.3 Validate on the production candidate (127.0.0.1:18362) that HTTP and client-WS requests land
  `upstream_response_model`, with `upstream_model_mismatch=false` on an exact match.
- [x] 5.4 Hot-swap the production binary and repoint the frontend release symlink.
- [x] 5.5 Verify the deployed admin usage API, the migration row, the public endpoints, and a real
  request through the public domain.
- [x] 5.6 Remove the canary container and its temporary env file.

### Deploy record

- Deployed binary sha256: `62d9a54ff164def4…` (release dir `0.1.391-usageaudit-62d9a54ff`), later superseded by
  `7cafaaf3ab36172d…` (`0.1.391-quotacontrol-7cafaaf3a`) when the quota-control removal shipped; both
  binaries carry the audit fields.
- Build recipe: HEAD plus the 51 files owned by the pricing and audit changes, `-tags embed`,
  `-trimpath`, `-ldflags "-X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380"`. The binary
  therefore carries the grok pricing additions and the fast-tier price entries as well as this change
  (`grok-imagine-image-2.0` and `cache_creation_input_token_cost_priority` are present in the deployed
  file). The quota-control removal was out of scope for this release and landed afterwards.
- Rollback binary: `/opt/sub2api/api/releases/0.1.391-acct-0383eff2f/sub2api.bak-usageaudit-20260922160223`
  (sha256 `03c524691bf55508…`, the previous live baseline)
- Rollback steps: `docker stop sub2api-api-candidate-0.1.391-acct-0383eff2f`, restore the binary
  backup, `docker start`; frontend `current` → `0.1.391-grok47-edb24612d` (now `rollback`).
- Frontend release: `0.1.391-usageaudit-c89474b36`, later followed by `0.1.391-quotacontrol-e50b9d83b`
  (the same audit UI rebuilt together with the quota-control removal).

### Verification limits

- The mismatch flag was exercised with matching models only (`upstream_response_model` == sent model).
  A genuine mismatch needs an upstream that answers with a different model than the one requested; it
  is covered by unit tests over the comparison helper.
- Rows settled by the pre-deploy binary keep `upstream_response_model` NULL, because the settlement
  payload of those attempts was serialized before the fields existed. The column is tri-state by
  design, so historical rows stay readable.
- Live per-minute evidence for the cut-over (rows / rows with the field): `03:57 7/1`, `04:00 1/0`,
  `04:02 1/1` (deploy at 04:02:23), `04:03 5/5`, `04:06 7/7`, `04:08 11/11`. The real Codex client on
  account 69 rides the WS v2 passthrough adapter (`ws_mode=passthrough`, `relay_turn_completed`), so
  the WS relay capture is exercised by production traffic, not only by the probe.

### Follow-ups

- The idle standby container `sub2api-api-candidate-0.1.391-routepair3-…` (ports 18360, binary sha
  `2646041b…`) is not referenced by the active nginx `sub2api_api` upstream, so it serves no traffic
  and was left untouched. Refresh its binary before ever promoting it above the live container.
- The admin usage CSV/table rendering of the new annotation should be eyeballed by an operator on the
  live UI; automated coverage is limited to the usage component suites.

## Notes

- Every live OpenAI upstream path populates the response model now: the Responses HTTP handlers, the
  two chat-completions handlers, the WS v2 forwarder, the client-WS ingress relay, the HTTP-to-WS
  bridge, and the WS v2 passthrough relay. The Claude/antigravity/gemini forwarders can start
  populating `ForwardResult.ResponseModel` later without any further schema or presentation work.
- The fields must be added to `domain.UsageLogSnapshot` alongside `domain.UsageLog`: the settlement
  payload is serialized for the authoritative billing transaction, so a missing snapshot field drops
  the value even though the repository insert already binds it.
- The mismatch column is deliberately not indexed yet; nothing filters on it. Add a partial index when
  a "only mismatched rows" filter is introduced.
- Deploy-gated: the migration is applied by the migration runner at startup, and the UI change needs a
  frontend release.
