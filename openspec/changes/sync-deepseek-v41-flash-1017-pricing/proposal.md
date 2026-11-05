# Change: Bill `deepseek-v4.1-flash-1017` at official DeepSeek V4.1 Flash rates

## Why

Upstream vendors serve DeepSeek V4.1 Flash under version-suffixed model IDs such as
`deepseek-v4.1-flash-1017`. Billing only priced two spellings today: the exact
`deepseek-v4.1-flash` (equality match) and the dash-form `deepseek-v4-flash*` family
(substring match). Every suffixed spelling of the dotted form therefore resolved to no
price at all:

- `deepseek-v4.1-flash-1017` → no pricing entry, no fallback match
- `deepseek-v4.1-flash-hc` → same; on 2026-10-02 one such request was recorded with
  `total_cost = 0`, i.e. the request was served for free

Depending on the path, an unpriced model is either rejected by the pricing-admission
gate (`503 PRICING_UNAVAILABLE`) or leaks through a channel/wildcard path and settles at
zero cost. Both outcomes are wrong for a SKU that is the same upstream model at the same
official price.

## What Changes

- Add one `fallback_pricing` entry for `deepseek-v4.1-flash-1017` with the official
  V4.1 Flash CNY rates (currency `CNY`, mirroring `deepseek-v4.1-flash`).
- Match the dotted `deepseek-v4.1-flash` family by prefix in the official-rate predicate
  so version-suffixed spellings inherit the official peak price and the off-peak
  half-price window instead of keeping whatever the snapshot says.
- Mirror the same price entry in the local model-pricing snapshot.
- Bump the model catalog version (`17` → `18`) and keep both `catalog.json` copies
  byte-identical.
- Update the stale DeepSeek V4 Flash expectations in the fallback-matching test to the
  current CNY rates; they still carried the pre-V4.1 `3 / 9 / 0.10` numbers and were
  failing before this change.

Out of scope: other vendor suffixes (`-hc`, `-uncensored`, …) stay fail-closed (no price)
until someone prices them deliberately, because a third-party finetune may not share the
official cost basis.

## Official Rates

CNY per one million tokens, from the DeepSeek official pricing page:

| Window | Input | Cache hit | Cache write | Output |
| --- | ---: | ---: | ---: | ---: |
| Peak (Mon–Fri, 01:00–04:00 and 06:00–10:00 UTC) | 2.00 | 0.04 | 2.00 | 8.00 |
| Off-peak (all other times) | 1.00 | 0.02 | 1.00 | 4.00 |

## Sources

- DeepSeek API pricing: <https://api-docs.deepseek.com/quick_start/pricing>

## Impact

- Affected specs: `model-catalog`.
- Affected code/data: both model catalog copies,
  `backend/resources/model-pricing/model_prices_and_context_window.json`,
  `backend/internal/service/billing_service.go`, and
  `backend/internal/service/billing_service_test.go`.
- No database migration and no API shape change. Models that already had prices keep
  exactly the same rates.
- Pricing currency stays `CNY` and is recorded as-is: settlement performs no FX
  conversion, matching every other DeepSeek model.
- The new price takes effect with the next binary deployment.

## Approval Gate

Implementation started on the explicit user request "deepseek-v4.1-flash-1017 同步官方这个价格"
and is complete; keep this document for review and archive it after deployment.
