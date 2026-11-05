# Change: Unify the pricing pipeline into five layers

## Why

Final customer price is currently produced by eleven independent layers with four
competing price sources (sell policy / pricing snapshot / catalog fallback / code pins).
The cost of that structure showed up three times in one day:

- `glm-5.3-abliterated`, `glm-5.3-flash-abliterated`, `minimax-m3.1-flash` were silently
  billed by an unrelated USD fallback (`$1.00/$3.20` and `$0.60/$2.40`) because
  `currency` is metadata only and billing never converts — a USD row is charged at par.
- `deepseek-v4.1-flash-abliterated` produced `503 PRICING_UNAVAILABLE` because the dotted
  V4.1 family was matched by equality only; the same class of gap once settled at zero
  cost (`deepseek-v4.1-flash-hc`, 2026-10-02).
- Repricing to a new exchange rate meant hand-recomputing eight rows, twice
  (6.67 → 7.0), with no record of which rate a given charge used.

None of these are model problems; they are structure problems. Cost basis and FX have no
home, prices have four homes, and the vendor peak window plus two price pins are Go
constants that cannot be configured or turned off.

## What Changes

Collapse the eleven layers into five, in one migration:

| Layer | Responsibility | Absorbs |
| --- | --- | --- |
| L1 cost | vendor price + discount + currency, margin only | new; replaces manual arithmetic |
| L2 price | one price table per model / context tier / billing mode, with `cny_native` / `fx_auto` / `manual` derivation | sell policy, snapshot, catalog fallback, code pins, intervals, long-context multipliers, group image/video prices |
| L3 modifier | one engine, scopes `vendor_time` / `request_tier` / `group_time` | DeepSeek peak window, `serviceTierCostMultiplier`, group peak surcharge |
| L4 rate | group `rate_multiplier` + per-user override | unchanged |
| L5 unit | balance denomination and recharge | unchanged (`balance-denomination`) |

Concretely:

- Add `price_cost` (vendor cost basis) and derive sell price from it per row, with FX and
  markup recorded per charge.
- Delete the separate pricing snapshot layer: the snapshot file becomes the built-in
  default data set in the same schema as DB price rows.
- Turn the DeepSeek official CNY pins and the GPT-5.6 cache-write rule into data, not code.
- Merge context-tier intervals and long-context multipliers into one context-tier list.
- Move image/video tier prices from `groups` into price rows (`billing_mode=image|video`).
- Make the vendor peak window data-driven and disableable per model pattern.
- Record `fx_mode`, `fx_rate` and applied modifiers on every usage row.

## Migration and cutover

Single migration, but never big-bang on the hot path:

1. Add nullable columns/tables; backfill every existing price row as `fx_mode=manual` so
   charged values stay byte-identical.
2. Import the known cost bases (venice `$0.33/$0.00825/$1.32`, refuseless list prices,
   hinoki `$0.80/$0.08/$1.20`) and switch those rows to `fx_auto` at the current rate —
   recomputed prices must equal today's values within 1e-9.
3. Shadow-run the unified resolver next to the legacy one, logging every difference;
   cut over with `PRICING_PIPELINE=unified` only after the diff log is clean.
4. Keep the legacy resolver selectable for one release for rollback.

## Impact

- Specs: new capability `pricing-pipeline`; `model-catalog` pricing requirements must keep
  holding (Qwen/MiMo exact CNY prices still resolve through L2 defaults).
- Code: resolver, billing service, channel/pricing repositories, admin handlers and DTOs,
  frontend pricing pages, migration + backfill tooling.
- Data: new tables/columns; `groups` loses 9 price columns; existing charge values are
  preserved by construction.
- Risk: this touches the billing hot path. Mitigated by the manual backfill, the shadow
  diff, and the pipeline switch; the eleven-layer behavior is reproduced exactly for
  existing rows, so only new-style rows use derivation.
- Out of scope: changing customer-facing prices, group multipliers, denomination, or the
  group peak surcharge semantics (that stays in `groups`, only the engine is unified).

## Approval Gate

Implementation must not begin until this change passes `openspec validate --strict` and the
user approves the proposal and design.
