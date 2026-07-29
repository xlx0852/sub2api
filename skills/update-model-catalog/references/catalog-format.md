# Catalog format

## Top-level document

```json
{
  "version": 2,
  "updated_at": "2026-07-29T06:00:00Z",
  "platforms": {},
  "fallback_pricing": {},
  "image_defaults": {},
  "ui_presets": {}
}
```

Required operational rules:

- `version`: positive integer; increment for every published change.
- `updated_at`: UTC RFC 3339 timestamp.
- `platforms`: non-empty object keyed by Sub2API platform name.
- `fallback_pricing`, `image_defaults`, `ui_presets`: objects, even when empty.

Supported first-party platform keys currently include `openai`, `anthropic`, `gemini`, `antigravity`, `grok`, and `bedrock`.

## Platform object

```json
{
  "default_test_model": "gpt-5.6-sol",
  "default_chat_model": "grok-4.5",
  "models": [],
  "aliases": {},
  "retired_ids": [],
  "default_mapping": {},
  "id_overrides": {},
  "id_reverse_overrides": {}
}
```

Fields are optional per platform:

- `default_test_model`: default account-test model.
- `default_chat_model`: default chat/upstream model where applicable.
- `models`: public metadata entries.
- `aliases`: client alias to a known model or mapping key.
- `retired_ids`: IDs hidden from selectable lists.
- `default_mapping`: client-visible ID to upstream model ID.
- `id_overrides`: outbound provider-specific ID rewrite.
- `id_reverse_overrides`: upstream ID to client-visible ID rewrite.

Use `default_mapping` for routing, not `aliases`. Keep both directions explicit when a provider needs reversible response normalization.

## Model entry

```json
{
  "id": "gpt-5.6-sol",
  "name": "models/gemini-3.5-flash",
  "display_name": "GPT-5.6 Sol",
  "object": "model",
  "owned_by": "openai",
  "type": "model",
  "created": 1785283200,
  "created_at": "2026-07-29",
  "family": "gpt-5.6",
  "is_reasoning": true,
  "supported_generation_methods": ["generateContent", "streamGenerateContent"],
  "media": {
    "image": false,
    "video": false,
    "audio": false
  },
  "flags": {
    "web_search": true
  }
}
```

Only `id` is universally required. Use other fields when known:

- `name`: provider-native resource name, mainly Gemini.
- `display_name`: user-facing label.
- `object`, `owned_by`, `type`, `created`, `created_at`: API/UI metadata.
- `family`: grouping and presentation family.
- `is_reasoning`: reasoning-model marker.
- `supported_generation_methods`: provider-native methods.
- `media`: supported media capabilities keyed by stable lowercase names.
- `flags`: boolean runtime capability hints such as web search.

Model IDs must be non-empty and unique inside one platform.

## Fallback pricing

```json
{
  "gpt-5.6-sol": {
    "input_cost_per_token": 0.00000125,
    "output_cost_per_token": 0.00001,
    "cache_read_input_token_cost": 0.000000125,
    "supports_cache_breakdown": true,
    "long_context_input_token_threshold": 200000,
    "long_context_input_cost_multiplier": 2,
    "long_context_output_multiplier": 1.5
  },
  "gpt-5.6-sol-latest": {
    "alias_of": "gpt-5.6-sol"
  }
}
```

Supported keys mirror `modelcatalog.PriceEntry`:

- `alias_of`
- `input_cost_per_token`
- `input_cost_per_token_priority`
- `image_input_cost_per_token`
- `output_cost_per_token`
- `output_cost_per_token_priority`
- `cache_creation_input_token_cost`
- `cache_read_input_token_cost`
- `cache_read_input_token_cost_priority`
- `supports_cache_breakdown`
- `long_context_input_token_threshold`
- `long_context_input_cost_multiplier`
- `long_context_output_multiplier`
- `output_cost_per_image`

All numeric prices and multipliers must be non-negative. `alias_of` must resolve to an existing entry and must not form a cycle. Prices are upstream cost fallbacks, not tenant/channel selling prices.

## Image defaults

```json
{
  "base_price_usd": 0.134,
  "size_multipliers": {
    "1K": 1,
    "2K": 1.5,
    "4K": 2
  }
}
```

## UI presets

```json
{
  "openai": [
    {
      "label": "GPT-5.6 Sol",
      "from": "gpt-5.6-sol",
      "to": "gpt-5.6-sol",
      "color": "blue"
    }
  ]
}
```

Each preset supports `label`, `from`, `to`, and optional `color`. Presets improve admin input; they do not grant routing capability.

## Publication invariants

- Project canonical and embedded files must be byte-identical.
- Shared `catalog.json` must be byte-identical to the project canonical file for the same version.
- `catalog.sha256` format is `<64 lowercase hex>  catalog.json` plus newline.
- `versions/<version>/catalog.json` and checksum are immutable after publication.
- Remote validation success does not prove actual provider availability; run account/upstream probes separately.
