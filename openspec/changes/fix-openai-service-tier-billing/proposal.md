# Change: Price OpenAI service tiers from the model's declared tier table

## Why

OpenAI's Fast mode (formerly Priority processing) is not a flat multiplier: the official price table
prices it per model, and Fast mode scales **cache writes** together with input. Two defects followed
from treating it as a blanket multiplier:

- The catalog fallback price entry could not express a Fast-tier cache-write price at all
  (`PriceEntry` had no `cache_creation_input_token_cost_priority`), so Fast requests billed
  cache writes at standard rates — a 50% under-charge on that line for the GPT-6 and GPT-5.6 families.
- Models with no Fast tier (for example the Daybreak cyber models and the `*-pro` variants, which the
  official Fast table omits) were still charged a flat 2x premium — an over-charge.

The same audit also found that the Daybreak cyber rows carried long-context fees that OpenAI does not
publish (the official table shows `-`), which the data fix removes.

## What Changes

- Add `cache_creation_input_token_cost_priority` to the catalog `PriceEntry` schema and map it in
  `priceEntryToModelPricing` so Fast-tier cache writes are represented end to end.
- Price Fast mode from the model's declared tier prices only; remove the blanket 2x fallback for
  `service_tier=priority` so models without a declared Fast tier are billed at standard rates.
- Keep Flex as the uniform 0.5x tier.
- Record the official Fast-tier cache-write prices for the GPT-6 and GPT-5.6 families in both catalog
  copies.
- Drop the long-context fields that were sourced from LiteLLM rather than OpenAI on the cyber rows.
- Bill by the service tier the upstream response reports instead of only the requested tier, so Fast
  requests that OpenAI downgrades under ramp-rate limits are charged standard rates.

## Impact

- Affected specs: `usage-billing-core` (service-tier pricing contract), `model-catalog` (price entry fields).
- Affected code: `backend/internal/pkg/modelcatalog/schema.go`,
  `backend/internal/service/billing_service.go`, both catalog copies,
  `backend/resources/model-pricing/model_prices_and_context_window.json`, and the OpenAI relay tier
  plumbing in `openai_gateway_response_handling.go`, `openai_gateway_pipeline_openai.go`,
  `openai_gateway_pipeline_chat.go`, `openai_gateway_pipeline_messages.go`,
  `openai_gateway_passthrough.go`, `openai_gateway_chat_completions_raw.go`.
- No API change and no database migration. Standard-tier and Flex pricing are unchanged; the
  behaviour change is limited to Fast/priority requests.
- Deploy-gated: the running production instance resolves OpenAI prices from the binary's fallback
  tables, so these fixes take effect only with a new binary.
