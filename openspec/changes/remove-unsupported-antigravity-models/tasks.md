# Tasks: Remove Unsupported Antigravity Models

- [x] Clean up `catalog.json` in resources and pkg <!-- id: 0 -->
  - [x] Remove `gemini-3-flash` and `gemini-3.1-flash-lite` from `antigravity.models`.
  - [x] Remove deprecated `gemini-2.5-*`, `gemini-3-pro-*`, `gemini-3-flash*`, and `gemini-3.1-flash-lite` from `antigravity.default_mapping`.
  - [x] Remove deprecated UI presets from `ui_presets.antigravity`.
  - [x] Anchor fallback Flash pricing to `gemini-3.8-flash` and remove obsolete pricing keys.
- [x] Clean up domain entity and service layers <!-- id: 1 -->
  - [x] Update `ensureAntigravityDefaultPassthroughs` in `account_entity.go`.
  - [x] Update legacy quota shape model fallback in `antigravity_quota_fetcher.go`.
- [x] Clean up frontend whitelist, presets, and modals <!-- id: 2 -->
  - [x] Remove obsolete models and presets from `useModelWhitelist.ts`.
  - [x] Remove obsolete models from `UseKeyModal.vue` and `AccountStatusIndicator.vue`.
- [x] Update tests and verify <!-- id: 3 -->
  - [x] Update `constants_test.go`, `models_test.go`, `antigravity_scope_test.go`, and frontend tests.
  - [x] Run backend tests and frontend tests to verify 0 regressions.
