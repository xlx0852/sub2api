# Change: Update Antigravity Gemini Model Catalog

## Why

The Antigravity client has updated its officially supported Gemini models to include Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro across reasoning effort levels (low, medium, high), alongside Flash Lite. The sub2api model catalog and presets are outdated and only list Gemini 3 Flash and Gemini 3.1 Pro Low/High, preventing users from selecting or routing to modern Gemini models.

In accordance with strict system boundaries, Claude models (`claude-*`) and image models remain completely excluded from the Antigravity scope.

## What Changes

- **Model Catalog**:
  - Add `gemini-3.8-flash`, `gemini-3.8-flash-high`, `gemini-3.7-flash`, `gemini-3.7-flash-high`, `gemini-3.6-flash`, `gemini-3.6-flash-high`, `gemini-3.1-pro`, and `gemini-3.1-flash-lite` to `antigravity.models` across both bundled catalog files (`backend/resources/model-catalog/catalog.json` and `backend/internal/pkg/modelcatalog/catalog.json`).
  - Synchronize both catalog files so embedded fallback catalog matches the resource catalog and completely excludes Claude models from `antigravity`.
  - Update `default_mapping` to support direct passthroughs and aliases for new 3.8, 3.7, 3.6, and 3.1 variants.
  - Update `fallback_pricing` with base pricing for Flash models and aliases for 3.8, 3.7, 3.6, and 3.1 effort variants.
  - Update `uipresets.antigravity` with preset mapping templates for 3.8, 3.7, 3.6, 3.1 Pro, and 2.5→3 Flash.
- **Account Domain Entity**:
  - Update `ensureAntigravityDefaultPassthroughs` in `account_entity.go` to include the modern Gemini models so accounts with custom mappings automatically receive passthrough support.
- **Frontend Presets & Helpers**:
  - Update `antigravityModels` in `frontend/src/composables/useModelWhitelist.ts` and add mapping preset buttons.
  - Add model abbreviation aliases in `frontend/src/components/account/AccountStatusIndicator.vue`.
  - Add limit, modality, and thinking configs in `frontend/src/components/keys/UseKeyModal.vue`.
- **Validation & Testing**:
  - Update `constants_test.go`, `models_test.go`, and `antigravity_scope_test.go` to verify inclusion of the modern Gemini models and exclusion of Claude and image models.

## Impact

- Affected specs: `antigravity`
- Affected code:
  - `backend/resources/model-catalog/catalog.json`
  - `backend/internal/pkg/modelcatalog/catalog.json`
  - `backend/internal/domain/account_entity.go`
  - `backend/internal/domain/constants_test.go`
  - `backend/internal/pkg/antigravity/models_test.go`
  - `backend/internal/service/antigravity_scope_test.go`
  - `frontend/src/composables/useModelWhitelist.ts`
  - `frontend/src/components/account/AccountStatusIndicator.vue`
  - `frontend/src/components/keys/UseKeyModal.vue`
