# Change: Move the model registry into the database and make prices reference it

## Why

A model currently "exists" in two unrelated places:

- `catalog.json` (compiled in, plus a copy under `resources/`) holds identity, metadata,
  aliases, UI presets, default chat/test models and `fallback_pricing`;
- the sell-price rows (`channel_model_pricing`) hold prices keyed by a **model-name array**.

Because the gateway does not treat `catalog.json` as a routing whitelist, a model that is
missing from it still runs as soon as a price row exists — that is how
`deepseek-v4.1-flash-abliterated`, `deepseek-v4-1-flash-1017-uncensored` and
`hinoki-offensive` went live this week. The consequences are all real, not theoretical:

- these models never appear in `/v1/models`, the model plaza or the API-key page;
- they have no `context_window` / `media` metadata, so no long-context behaviour and no
  vision gating;
- request aliases (`kimi3.1`, Claude short ids, `-1017`/`-hc` suffixes) are resolved by a
  compiled table, so a suffixed model name silently misses both pricing and mapping — the
  cause of the 503s and the zero-cost charge fixed earlier today;
- deleting or unbinding a price row silently un-lists the model instead of falling back.

Meanwhile the price rows cannot be the registry: they carry none of the fields above, and a
single row would have to duplicate model metadata once per group.

## What Changes

Split the responsibilities into three tables and make the price rows reference the registry
by foreign key instead of by name.

| Table | Cardinality | Holds |
| --- | --- | --- |
| `models` | 1 row per model | id (canonical), platform, display name, family, context window, media, reasoning flag, enabled/retired, default chat/test flags, version |
| `model_aliases` | 1 row per alias | alias → model id (+ platform, kind: alias / id override / retired id) |
| `model_prices` | 1 row per (model × group) | prices, billing mode, currency, `fx_mode` + cost basis, priority prices, context/image tiers, `group_id NULL` = the platform default (fallback) |

Consequences:

- **one place to create a model, one place to price it**: the new「模型」page owns identity,
  metadata, aliases and the default price, and its detail view shows a per-group price table;
- **unregistered models cannot be priced**: saving a price for an unknown `model_id` fails
  with `MODEL_NOT_REGISTERED`, so "exists" has exactly one definition;
- **the dotted/suffixed alias trap disappears** from the data model: prices hang off a
  canonical id, and the alias table maps every spelling onto it;
- **the fallback becomes explicit data** (`group_id IS NULL`), replacing
  `fallback_pricing` in the catalog file;
- **priority pricing stops being lost**: price rows gain `input_price_priority` /
  `output_price_priority`, which the current sell rows cannot express.

The in-memory `modelcatalog.Catalog` struct stays as the read path, so the ~15 call sites
(`PublicView`, `OpenAIModels`, `AnthropicIDOverrides`, default test/chat models, …) do not
change: only the loader gains a database source with the file as fallback.

## Compatibility and migration

1. New migration creates the three tables, seeds them from both `catalog.json` copies and
   from the two live sell tables (18 model names → per-group rows), preserving `fx_auto`,
   `cost_id`, `fx_rate` and `markup`.
2. `MODEL_REGISTRY_SOURCE=file|dual|db` and `MODEL_PRICING_SOURCE=legacy|dual|model_prices`
   switches (settings-backed, hot, same pattern as `pricing_pipeline`):
   - `dual` loads/derives from both and only logs differences;
   - `db` / `model_prices` becomes the source of truth.
3. `channel_model_pricing` stays readable for one release so rollback is a switch flip.
4. Deleting a group price row now means "fall back to the default price"; deleting the
   default row means fail-closed and the model is flagged red in the admin list.

## Impact

- Specs: new capability `model-registry`; `model-catalog` and `pricing-pipeline` requirements
  must keep resolving the same values (the seeded defaults reproduce them exactly).
- Code: `internal/pkg/modelcatalog` loader, `ModelPricingResolver`, billing fallback lookup,
  channel repository (read path), admin handlers/routes, two admin pages.
- Data: three new tables, one migration that seeds them, no destructive change to existing
  tables; `usage_logs` untouched.
- Risk: alias/id normalization sits on the request hot path. Mitigated by loading the registry
  into memory (same as today), by `dual` mode diffing against the compiled table, and by the
  pricing/registry switches.
- Out of scope: dropping `channel_model_pricing`, moving image/video prices out of `groups`,
  and changing any customer price.

## Approval Gate

Implementation must not begin until this change passes `openspec validate --strict` and the
user approves. Slice 1 (additive schema + seed + dual-read, no behaviour change) can ship on
its own.
