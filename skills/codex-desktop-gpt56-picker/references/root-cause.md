# Root Cause: Codex Desktop missing GPT-5.6 in picker

## Two pipelines

### Inference path (can work without picker)

User/config forces `model = gpt-5.6-sol` → request goes to provider `base_url` → succeeds if gateway supports the model.

### Discovery + UI path (what the picker needs)

```
provider Codex manifest (client_version)
        │
        ▼
~/.codex/models_cache.json  (+ model_catalog.json)
        │
        ▼
codex app-server  model/list RPC
        │
        ▼
Desktop list-models-for-host
        │
        ▼
Statsig dynamic config 107580212  → filter Jv({availableModels, useHiddenModels, models})
        │
        ▼
Model picker UI
```

## Layer A — API Key mode freezes discovery

With custom provider + API key auth, Desktop often **does not refresh** the online model catalog. It falls back to:

1. `~/.codex/models_cache.json` if present
2. else bundled catalog inside the Desktop/codex binary (may include GPT-5.2, may miss 5.6)

5.6 entries require `minimal_client_version >= 0.144.0` style clients.

Useful probe endpoints against the gateway:

- `GET /models?client_version=0.144.0`
- `GET /v1/models?client_version=0.144.0`
- `GET /backend-api/codex/models?client_version=0.144.0`

Sparse OpenAI `/v1/models` (`{id, object}`) is **not enough** for a rich Desktop catalog. Prefer Codex manifests with `display_name`, `supported_reasoning_levels`, `visibility`, etc.

## Layer B — Statsig allowlist

Desktop reads dynamic config id **`107580212`** (observed in app.asar). Typical live value under API-key / incomplete rollout:

```json
{
  "use_hidden_models": true,
  "default_model": "gpt-5.4",
  "available_models": [
    "gpt-5.5",
    "gpt-5.4",
    "gpt-5.4-mini",
    "gpt-5.3-codex",
    "gpt-5.2"
  ]
}
```

Filter behavior:

- `use_hidden_models === true` → show only models whose slug is in `available_models`
- `use_hidden_models === false` → show non-hidden models from `model/list`

Client exposure for CDP patch:

```js
window.__STATSIG__.firstInstance.getDynamicConfig('107580212').value
```

## Dual menus in UI

1. **Simple / power presets** — may hardcode Sol/Terra/Luna chips even when advanced list is filtered.
2. **Advanced full model list** — fully gated by Statsig allowlist.

So users can screenshot a power menu with 5.6 while advanced list still shows 5.2 + 自定义.

## Why configured 5.6 becomes 自定义

If `config.toml` has `model = "gpt-5.6-sol"` but the slug is not in the filtered list, Desktop treats it as a custom override label (自定义 / Custom) instead of a first-class catalog entry.

## Wrong app trap

| App | Bundle | Uses ~/.codex model/list + Statsig 107580212 |
|-----|--------|-----------------------------------------------|
| Codex Desktop (`ChatGPT.app` 26.x) | `com.openai.codex` | Yes |
| ChatGPT Classic | `com.openai.chat` | No |

## Persistence

| Artifact | Survives restart |
|----------|------------------|
| `models_cache.json` / `model_catalog.json` | Yes |
| CDP Statsig monkeypatch | No |
| Official OpenAI allowlist update | Yes (when shipped) |
