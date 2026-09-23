## 1. Catalog

- [x] 1.1 Add `gpt-6-sol` and `gpt-6-luna` to `platforms.openai.models` in the embedded catalog.
- [x] 1.2 Mirror the same entries in the resources catalog copy.
- [x] 1.3 Add `ui_presets.openai` entries for both models and drop the duplicated GPT-5.5 preset.
- [x] 1.4 Bump catalog `version` / `updated_at` (11 -> 12).

## 2. Pricing

- [x] 2.1 Add static fallback pricing for `gpt-6-sol` and `gpt-6-luna` in `pricing_service.go`.
- [x] 2.2 Wire `gpt-6-sol*` / `gpt-6-luna*` prefixes in `matchOpenAIModel` ahead of the default fallback.
- [x] 2.3 Add official rows to `model_prices_and_context_window.json` (GPT-6 Sol/Luna with priority,
  batch, flex and above-272K fields; Daybreak cyber models).
- [x] 2.4 Add `fallback_pricing` entries for `gpt-6-sol` and `gpt-6-luna` in both catalogs.

## 3. Pool rollout

- [x] 3.1 Extend the GPT Pro account model mapping with `gpt-6-sol` and `gpt-6-luna`.
  - Accounts 69 (`GPT-自有-1`) and 120 (`GPT-外借-4`) both accept the models upstream; each mapping
    grew from 9 to 11 entries with OAuth tokens preserved.
  - Account 126 (`Devin` bridge) advertises the models via `/v1/models` but rejects real requests with
    upstream 403, so it was reverted to `{"swe-2"}` and its 403 cooldown was cleared.
- [x] 3.2 Verify `/v1/models` exposes both models for the GPT Pro group.
- [x] 3.3 Verify a live request routes and bills instead of falling back to the default model.

## 4. Verification

- [x] 4.1 Run targeted Go tests for catalog and pricing packages (plus temporary assertion tests in a
  clean worktree, since the working tree has an unrelated in-flight compile break in
  `internal/service/user_msg_queue_service.go`).
- [x] 4.2 Run `openspec validate update-openai-gpt6-sol-luna-catalog --strict`.
- [x] 4.3 Run `git diff --check`.

## 5. Production activation (ops override, no binary deploy)

- [x] 5.1 Identify that the live instance runs entirely on binary static fallbacks: its pricing service
  fails to initialise (`fallback file not found`) and both `PRICING_DATA_DIR` and
  `MODEL_CATALOG_DATA_DIR` point at empty directories.
- [x] 5.2 Install an ops-override catalog derived from the live active catalog plus the two models and
  their official prices at
  `/opt/sub2api/api/releases/0.1.383-external/resources/model-catalog/catalog.json`.
- [x] 5.3 Restart the live container and confirm health.
- [x] 5.4 Confirm billing now resolves at official rates and no `pricing_missing_record_zero_cost`
  warnings are emitted after restart.
- [x] 5.5 Revert the earlier ineffective edits to `/opt/sub2api/data/model_pricing.json` and
  `/opt/sub2api/data/model-catalog/catalog.json` (those paths are not read by the live container).

## Notes

- Rollback for the production override: delete
  `/opt/sub2api/api/releases/0.1.383-external/resources/model-catalog/catalog.json` and restart the
  container to fall back to the embedded catalog (removes the two models and their pricing).
- The ops-override catalog is a stopgap. Once a binary carrying this change is deployed, the embedded
  catalog supersedes it and the override file should be removed to avoid drift.
