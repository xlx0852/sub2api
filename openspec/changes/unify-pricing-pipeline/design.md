# Design: unified pricing pipeline

## 1. Target formula

```
charged_units(scope) =
    L2.price(model, context_class, billing_mode)          -- CNY per unit of content
  × Π L3.modifiers(vendor_time, request_tier, group_time)  -- dimensionless
  × L4.group_rate                                         -- dimensionless
  ÷ L5.unit_value                                         -- currency per balance unit

L1 only feeds L2 and never reaches the charge path.
```

`L2.price` is either authored (`manual`, `cny_native`) or derived
(`fx_auto = L1.cost × fx_rate × (1 + markup)`).

## 2. Data model

### 2.1 L1 — cost basis (new)

```sql
CREATE TABLE price_costs (
  id            bigserial PRIMARY KEY,
  vendor        text NOT NULL,            -- 'venice' | 'refuseless' | 'hinoki' | 'manual'
  model_pattern text NOT NULL,            -- exact id or prefix wildcard
  currency      text NOT NULL CHECK (currency IN ('USD','CNY')),
  input_per_mtok  numeric(20,10) NOT NULL,
  cached_per_mtok numeric(20,10),
  output_per_mtok numeric(20,10),
  discount_pct    numeric(6,3) NOT NULL DEFAULT 0,
  observed_at     timestamptz NOT NULL,
  source_url      text NOT NULL DEFAULT '',
  notes           text NOT NULL DEFAULT '',
  UNIQUE (vendor, model_pattern)
);
```

Cost rows are informational for margin reporting and for `fx_auto` derivation. Discount is
applied at derivation time (`cost × (1 - discount_pct/100)`), so ending a promotion is one
number, not a price rewrite.

### 2.2 L2 — price rows (extend `channel_model_pricing`)

```sql
ALTER TABLE channel_model_pricing
  ADD COLUMN fx_mode      text NOT NULL DEFAULT 'manual'
             CHECK (fx_mode IN ('cny_native','fx_auto','manual')),
  ADD COLUMN cost_id      bigint REFERENCES price_costs(id),
  ADD COLUMN markup_pct   numeric(6,3) NOT NULL DEFAULT 0,
  ADD COLUMN fx_rate      numeric(12,6),        -- frozen rate for fx_auto rows
  ADD COLUMN context_tiers jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN image_tiers   jsonb NOT NULL DEFAULT '[]'::jsonb;
```

- `input_price/output_price/cache_*` remain the authoritative charged values for every mode.
  For `fx_auto` rows they are recomputed whenever cost, discount, markup or rate changes.
- `context_tiers` replaces both `channel_pricing_intervals` rows and the longitude-context
  thresholds:

```json
[{"min":0,"max":272000,"multiplier":1.0},
 {"min":272000,"max":null,"input":4.0,"output":30.0,"cache_read":0.6}]
```

Absolute prices win over multipliers when both are present; a context class is resolved once
per request and is then a plain `L2.price`.

- `image_tiers` absorbs `groups.image_price_1k/2k/4k`, `video_price_480p/720p/1080p` and the
  batch-image multipliers (`billing_mode=image|video` rows). `groups` keeps only switches.
- `currency` becomes display-only. No code path may treat a USD row as chargeable.
- `billing_mode` values: `token | per_request | image | video`.

### 2.3 L3 — modifier engine

```sql
CREATE TABLE price_modifiers (
  id          bigserial PRIMARY KEY,
  scope       text NOT NULL CHECK (scope IN ('vendor_time','request_tier','group_time')),
  target      text NOT NULL,          -- model pattern (vendor/request) or group id (group)
  kind        text NOT NULL,          -- 'window' | 'tier'
  when_mode   text NOT NULL DEFAULT 'inside',  -- 'inside' | 'outside'
  window_start text, window_end text, tz text NOT NULL DEFAULT 'UTC',
  weekdays    smallint[],             -- NULL = every day
  tier_value  text,                   -- e.g. 'flex', 'priority'
  multiplier  numeric(10,6) NOT NULL,
  enabled     boolean NOT NULL DEFAULT true,
  priority    int NOT NULL DEFAULT 100,
  notes       text NOT NULL DEFAULT ''
);
```

Seed rows replace today's constants:

| scope | target | definition | replaces |
| --- | --- | --- | --- |
| vendor_time | `deepseek-v4*` | weekdays Mon–Fri, 01:00–04:00 + 06:00–10:00 UTC, `outside`, ×0.5 | `deepSeekOffPeakMultiplier`, `isDeepSeekPeakTime` |
| request_tier | `*` | `tier_value=flex`, ×0.5 | `serviceTierCostMultiplier` |
| request_tier | `*` | `tier_value=priority`, ×1.0 (price comes from the priority tier list) | implicit constant |

`group_time` keeps reading `groups.peak_*` through the same interface (it is already
UI-managed and per-group); moving those four columns into `price_modifiers` buys nothing and
would churn the admin UI. The engine exposes one `ApplyModifiers(price, ctx) (price, applied[])`.

Ordering is fixed and auditable: vendor_time → request_tier → group_time, then L4, then L5.
Every applied modifier is recorded by id.

### 2.4 Removing the extra sources

- `isDeepSeekV4OfficialModel` / `applyDeepSeekOfficialPeakPricing` → data (cost row for
  `deepseek-v4*` + the vendor_time modifier). The pin exists to defend against a stale
  snapshot; after this change the built-in defaults ARE the reviewed data, so the pin is
  redundant.
- GPT-5.6 cache-write `1.25×` → an explicit `cache_write` price in the defaults.
- The bundled `model_prices_and_context_window.json` and the data-dir
  `model_pricing.json` become one built-in defaults document in the L2 schema, with the
  existing remote-sync mechanism kept for hot updates.

## 3. Resolution algorithm

```
resolve(model, group, tokens, tier, now):
  price = db_price_row(group, model)                      # L2 override (sell policy)
       ?? builtin_default(model)                          # L2 defaults
       ?? error(pricing_unavailable)                      # fail-closed, unchanged
  price = applyContextTier(price, tokens.context)
  price = derive(price)                                   # fx_auto only
  price, applied = modifiers.Apply(price, model, group, tier, now)
  rate  = group.rate × userOverride(user, group)           # L4
  return charge(price, rate, unitValue)                    # L5
```

`derive()` runs on read (cheap, cached per price row version) so a rate change does not
require writing every row; the written `input_price/...` columns stay as the last derived
snapshot for display and for `manual` rows.

## 4. Observability

- `usage_logs` adds `fx_mode`, `fx_rate`, `modifier_ids[]`, `unit_value`.
- `GET /admin/groups/:id/pricing-summary` reports cost → derived sell → effective
  (sell × group rate) → gross margin per model.
- A nightly job compares `price_costs` against the last observed vendor price and flags
  drift (vendors change prices silently, as seen with refuseless promotions).

## 5. Migration

1. `ALTER TABLE ... ADD COLUMN` (nullable / defaulted) — no rewrite on the hot path.
2. Backfill: every existing price row gets `fx_mode='manual'`; charged values unchanged.
3. Import `price_costs` for the known USD vendors, then flip exactly these rows to
   `fx_auto` with `fx_rate=7.0`; assert recomputed = existing within 1e-9, else abort.
4. Seed `price_modifiers` from the constants above; move GPT-5.6 cache-write into defaults.
5. Move group image/video prices into `image_tiers`; verification query asserts the
   effective price for each (group, model, tier) triple is unchanged before the columns are
   dropped in a later release.
6. Shadow mode: `PRICING_PIPELINE=shadow` computes both pipelines and logs deltas per
   request without affecting the charge. Cut over with `=unified` after 24 h of zero deltas
   beyond rounding; keep `=legacy` for rollback one release.
7. Rollback: flip `PRICING_PIPELINE=legacy`; all new columns are additive, so no data
   rollback is needed.

## 6. Compatibility

- `balance-denomination` (1 unit per 1 principal) is untouched: L5 stays.
- Existing exact-CNY requirements in `model-catalog` keep holding because the defaults
  document carries the same values.
- Peak/off-peak for DeepSeek outside a sell policy is preserved by the seeded modifier.
- Requests with no price still fail closed with `503 PRICING_UNAVAILABLE`.

## 7. Rejected alternatives

- Two price columns per model (`price_usd`, `price_cny`): duplicates truth and still needs
  a rule for which one charges.
- Global FX setting only: cannot express "some models must not be converted", which is
  exactly today's requirement.
- Moving group peak surcharge into `price_modifiers`: churn without benefit; the engine
  unifies the code path instead.
