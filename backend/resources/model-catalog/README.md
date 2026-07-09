# Model Catalog

First-party model lists, default mappings, retired IDs, fallback pricing, and UI presets live in `catalog.json`.

## How it works

1. **Embedded copy** is compiled into the binary (`internal/pkg/modelcatalog/catalog.json` via `//go:embed`).
2. At process start, the loader prefers a filesystem override when present:
   - `resources/model-catalog/catalog.json`
   - `backend/resources/model-catalog/catalog.json`
3. Package facades (`openai.DefaultModels`, `xai.DefaultModelMapping`, `domain.DefaultAntigravityModelMapping`, billing fallbacks, …) read from this registry.
4. Admin API: `GET /api/v1/admin/model-catalog` exposes the document for the admin UI.

LiteLLM remote pricing (`resources/model-pricing/`) remains the primary dynamic price source. Values under `fallback_pricing` are used only when LiteLLM has no usable entry.

## Add or update a model

1. Edit `catalog.json` (keep both copies in sync, or regenerate the embed copy from this file).
2. Restart the server (no hot-reload in v1).
3. Optional: update UI preset chips under `ui_presets`.

```bash
# Keep embed copy aligned after editing the resources file:
cp backend/resources/model-catalog/catalog.json backend/internal/pkg/modelcatalog/catalog.json
```

## Schema highlights

| Field | Purpose |
|-------|---------|
| `platforms.<name>.models` | Default model catalog for that platform |
| `platforms.*.default_mapping` | Account empty-mapping defaults (Grok, Antigravity, Bedrock) |
| `platforms.openai.retired_ids` | Hidden from selectable lists |
| `fallback_pricing` | USD per-token fallback (LiteLLM-compatible keys); `alias_of` for shared prices |
| `image_defaults` | Default per-image unit pricing |
| `ui_presets` | Admin mapping chip suggestions |

## Notes

- Do not put secrets in this file.
- Tenant overrides (channel pricing, account `model_mapping`, group model lists) stay in the database.
- Third-party channel model name hints in the frontend (智谱 / 通义 / …) are still local TS lists in v1.
