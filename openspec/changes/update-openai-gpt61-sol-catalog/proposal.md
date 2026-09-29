# Change: Register OpenAI GPT-6.1 Sol and repair fallback pricing aliases

## Why

OpenAI's official model and pricing pages now list `gpt-6.1-sol` (display name "GPT-6.1 Sol") as
the current Sol model of the GPT-6 family; the older `gpt-6-sol` page points readers to it. Sub2API
knows neither the model ID nor its prices, so requests for `gpt-6.1-sol` fall through to the default
test model price (`gpt-5.5`) and the model is missing from model discovery and admin presets.

Prices were verified against OpenAI's own pages (no third-party data):

- `https://developers.openai.com/api/docs/models/gpt-6.1-sol.md` (fetched 2026-09-29, redirect from
  `platform.openai.com/docs/models/gpt-6.1-sol.md`): model ID `gpt-6.1-sol`,
  `reasoning.effort` = low/medium(default)/high/xhigh/max with `none` and `minimal` unsupported,
  Responses API required for tool calling, 1,050,000 context / 128,000 max output, knowledge cutoff
  Apr 30, 2026, Fast mode unavailable under EU data residency.
- `https://developers.openai.com/api/docs/pricing.md` (fetched 2026-09-29, sha256
  `b271bf08244492bcaa2a169a86c1c486ea8f4b1713ee990ce05b49435cd8c510`): standard tier
  `$2.00 | $0.10 | $2.50 | $10.00` (`<272K`) and `$4.00 | $0.20 | $5.00 | $15.00` (`>272K`);
  Batch and Flex `$1.00 | $0.05 | $1.25 | $5.00`; Fast (priority) `$4.00 | $0.20 | $5.00 | $20.00`.
  Compared with `gpt-6-sol`, the only price difference is the cached-input rate (5% vs 10%).

While synchronizing the embedded catalog copy for this change, the catalog verifier failed on a
pre-existing defect: `fallback_pricing["gpt-5.5"]` (and therefore `gpt-5.5-pro`) aliased itself
(`alias_of: gpt-5.5`). Commit `25a599749` ("retire gpt-5.4 product surface and promo codes") renamed
the GPT-5.4 product surface with a mechanical rewrite that turned the previous `alias_of: gpt-5.4`
into a self-reference, and dropped the `gpt-5.4*` fallback entries that `billing_service.go` still
looks up. The live binary never noticed because the embedded copy had not been regenerated yet, so
the resources copy and the embedded copy diverged. This change repairs that divergence with
behavior-preserving values and reports the underlying pricing question instead of silently
redefining it.

## What Changes

- Add `gpt-6.1-sol` to `platforms.openai.models` (both catalog copies) with display name
  "GPT-6.1 Sol" and `use_responses_lite: true`, plus a matching `ui_presets.openai` entry.
- Add `fallback_pricing["gpt-6.1-sol"]` with the official standard rates, the 2x priority tier, the
  272,000-token long-context tier (2x input / 1.5x output) and cache breakdown support.
- Add a static fallback in `PricingService` for `gpt-6.1-sol`, and match the `gpt-6.1-sol` prefix
  ahead of `gpt-6-sol` so unknown variants never inherit the 10% cached-input rate of `gpt-6-sol`.
- Add the official `gpt-6.1-sol` rows to the pricing mirror
  (`model_prices_and_context_window.json`): standard, above-272K, batches, flex and priority.
- Repair the `fallback_pricing` alias graph: restore the `gpt-5.4`, `gpt-5.4-mini` and
  `gpt-5.4-nano` entries that `25a599749` removed and point `gpt-5.5` back at `gpt-5.4`, restoring
  the cycle-free chain and the exact fallback prices the last shipped embedded catalog produced for
  `gpt-5.4*`, `gpt-5.5` and `gpt-5.5-pro`.
- Bump the catalog `version` (15 -> 16), regenerate the embedded copy byte-identically, and publish
  the validated document to the shared `xlx0852/model-catalog` repository.
- Add regression tests covering the GPT-6.1 Sol catalog entry, its static fallback pricing, the
  mirror rates and the preserved `gpt-6-sol` cached-input rate.

## Impact

- Affected specs: `model-catalog`; `usage-billing-core` (pricing resolution only, no schema or
  settlement change).
- Affected code: `backend/resources/model-catalog/catalog.json`,
  `backend/internal/pkg/modelcatalog/catalog.json`,
  `backend/resources/model-pricing/model_prices_and_context_window.json`,
  `backend/internal/service/pricing_service.go`, plus the new tests.
- Live routing unchanged: `gpt-6.1-sol` is added to the catalog only; making it routable requires
  an account-side model mapping, which is deliberately not part of this change.
- Fallback prices for existing models are unchanged: `gpt-5.4*`, `gpt-5.5` and `gpt-5.5-pro`
  resolve to the same effective values the previously shipped embedded catalog produced.
- No database migration, no breaking API change.
- Follow-up (not part of this change): the mirror's authoritative, officially verified prices for
  `gpt-5.5` ($5.00/$30.00 per MTok) and `gpt-5.5-pro` ($30.00/$180.00) are far above the legacy
  GPT-5.4 rates those fallback entries still resolve to. That fallback is only used when the pricing
  service has no data for the model, but the discrepancy should be decided explicitly rather than
  rewritten by a mechanical rename.
