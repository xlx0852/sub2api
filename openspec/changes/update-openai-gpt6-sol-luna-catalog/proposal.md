# Change: Register OpenAI GPT-6 Sol/Luna and Daybreak pricing

## Why

OpenAI's official pricing and model pages now list `gpt-6-sol` and `gpt-6-luna` as part of the
GPT-6 family, alongside Daybreak cyber models (`gpt-5.6-cyber`, `gpt-5.5-cyber`) and new
realtime/audio endpoints. Sub2API's catalog, pricing mirror, and static pricing fallback do not
know `gpt-6-sol`/`gpt-6-luna`, so requests for them fall through to the default test model price
(`gpt-5.5`), which mis-bills every call and hides the models from model discovery.

## What Changes

- Add `gpt-6-sol` and `gpt-6-luna` to the OpenAI catalog in both the embedded and resources
  copies, with matching UI presets.
- Add official GPT-6 Sol/Luna pricing (standard, priority, batch/flex, and the >272K
  long-context multiplier) as static fallback pricing in `PricingService` so unknown
  `gpt-6-sol*` / `gpt-6-luna*` variants never inherit another model's rate.
- Add official extended pricing rows to the pricing mirror (`model_prices_and_context_window.json`)
  and the catalog `fallback_pricing` block for GPT-6 Sol/Luna and the Daybreak cyber models.
- Expose the two models through the GPT Pro pool by extending the account model mapping on the
  account whose upstream advertises support.

## Impact

- Affected specs: `model-catalog`, `usage-billing-core` (pricing resolution only; no schema or
  settlement change).
- Affected code: `backend/internal/pkg/modelcatalog/catalog.json`,
  `backend/resources/model-catalog/catalog.json`,
  `backend/resources/model-pricing/model_prices_and_context_window.json`,
  `backend/internal/service/pricing_service.go`.
- Affected data: devin-bridge account model mapping (operational, no migration).
- No database migration. No breaking API change. Existing model IDs and prices are unchanged.
