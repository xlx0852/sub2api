# Change: Remove Unsupported Antigravity Models

## Why

The modern Antigravity client only supports four Gemini models: Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro (with low/medium/high reasoning effort). Legacy models such as `gemini-3-flash`, `gemini-3.1-flash-lite`, `gemini-2.5-*`, and `gemini-3-pro-*` are no longer supported by the client. Cleaning them up prevents obsolete model selection, stale mappings, and confusing options in the admin dashboard and API key interfaces.

## What Changes

- **Catalog Cleanup**:
  - Remove `gemini-3-flash` and `gemini-3.1-flash-lite` from `antigravity.models` across both `backend/resources/model-catalog/catalog.json` and `backend/internal/pkg/modelcatalog/catalog.json`.
  - Remove obsolete legacy mappings (`gemini-2.5-*`, `gemini-3-pro-*`, `gemini-3-flash*`, `gemini-3.1-flash-lite`) from `antigravity.default_mapping`.
  - Remove obsolete UI presets (`Gemini 3-Flash透传`, `Gemini 2.5→3 Flash`, `Gemini 3.1 Flash Lite透传`) from `ui_presets.antigravity`.
  - Re-anchor fallback Flash pricing to `gemini-3.8-flash` and remove obsolete pricing keys.
- **Backend Domain & Service**:
  - Remove `gemini-3-flash` and `gemini-3.1-flash-lite` from `ensureAntigravityDefaultPassthroughs` in `account_entity.go`.
  - Update legacy model quota fallbacks in `antigravity_quota_fetcher.go` to use `gemini-3.8-flash`.
- **Frontend Whitelist & Modals**:
  - Remove obsolete models from `antigravityModels` and presets in `frontend/src/composables/useModelWhitelist.ts`.
  - Clean up obsolete model entries from `UseKeyModal.vue` and `AccountStatusIndicator.vue`.
- **Testing & Verification**:
  - Update tests to ensure only client-supported Gemini models are published and tested.

## Impact

- Affected specs: `antigravity`
- Affected code:
  - `backend/resources/model-catalog/catalog.json`
  - `backend/internal/pkg/modelcatalog/catalog.json`
  - `backend/internal/domain/account_entity.go`
  - `backend/internal/domain/constants_test.go`
  - `backend/internal/pkg/antigravity/models_test.go`
  - `backend/internal/service/antigravity_quota_fetcher.go`
  - `backend/internal/service/antigravity_scope_test.go`
  - `frontend/src/composables/useModelWhitelist.ts`
  - `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`
  - `frontend/src/components/keys/UseKeyModal.vue`
  - `frontend/src/components/account/AccountStatusIndicator.vue`
