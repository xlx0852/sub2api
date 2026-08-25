# Change: Add DeepSeek V4 Flash Vision Exp

## Why

DeepSeek has released `deepseek-v4-flash-vision-exp`, an experimental multimodal model that accepts image input. The gateway already supports DeepSeek chat routing, but the new model is absent from the first-party catalog, billing metadata, and client model presets.

## What Changes

- Add the model to both bundled first-party model catalog copies.
- Mark it as a reasoning, vision-capable DeepSeek chat model with a self-mapping.
- Add pricing metadata using the existing DeepSeek V4 Flash pricing baseline.
- Expose the model in the frontend DeepSeek whitelist and preset mappings.
- Add catalog and frontend regression coverage.

## Impact

- Affected specs: model catalog and model selection.
- Affected code: `backend/resources/model-catalog/catalog.json`, `backend/internal/pkg/modelcatalog/catalog.json`, `backend/data/model_pricing.json`, and `frontend/src/composables/useModelWhitelist.ts`.
- No database migration, provider adapter, or API contract change.
