# Tasks: Update Antigravity Gemini Model Catalog

- [x] Update `backend/resources/model-catalog/catalog.json` and `backend/internal/pkg/modelcatalog/catalog.json` <!-- id: 0 -->
  - [x] Add `gemini-3.8-flash`, `gemini-3.8-flash-high`, `gemini-3.7-flash`, `gemini-3.7-flash-high`, `gemini-3.6-flash`, `gemini-3.6-flash-high`, `gemini-3.1-pro`, and `gemini-3.1-flash-lite` to `antigravity.models`.
  - [x] Add default mappings and effort-level aliases in `antigravity.default_mapping`.
  - [x] Add fallback pricing entries and aliases for Flash and Pro models in `fallback_pricing`.
  - [x] Add UI preset mapping templates in `uipresets.antigravity`.
  - [x] Synchronize embedded `catalog.json` with resource catalog ensuring no Claude or image models remain under `antigravity`.
- [x] Update account entity default passthroughs <!-- id: 1 -->
  - [x] Include modern Gemini models in `ensureAntigravityDefaultPassthroughs` in `backend/internal/domain/account_entity.go`.
- [x] Update frontend models, presets, and UI indicators <!-- id: 2 -->
  - [x] Add modern Gemini models to `antigravityModels` and add preset mapping templates in `frontend/src/composables/useModelWhitelist.ts`.
  - [x] Add short aliases in `frontend/src/components/account/AccountStatusIndicator.vue`.
  - [x] Add key usage configs for the new models in `frontend/src/components/keys/UseKeyModal.vue`.
- [x] Update tests and verify <!-- id: 3 -->
  - [x] Update `backend/internal/domain/constants_test.go` to test modern Gemini mappings and verify non-Gemini exclusion.
  - [x] Verify `backend/internal/pkg/antigravity/models_test.go` and `backend/internal/service/antigravity_scope_test.go`.
  - [x] Run full test suite for `internal/pkg/modelcatalog`, `internal/domain`, `internal/pkg/antigravity`, and `internal/service`.
