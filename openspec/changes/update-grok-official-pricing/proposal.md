# Change: Align xAI Grok pricing with the official rate card

## Why

An audit of the xAI rate card (docs.x.ai API Pricing) against our Grok pricing found the catalogue is
incomplete and in places wrong. The most serious consequence is long-context under-billing: xAI bills
**all tokens in a request at the long-context rate once the prompt reaches 200k tokens**, but for
`grok-4.3`, `grok-4.5`, `grok-build-0.1` and the three `grok-4.20-*` models that tier is missing or has
a threshold equal to the context window, so long prompts are billed at short-context rates.

## What Changes

- Correct the Grok text rate card so the long-context threshold is 200,000 tokens with a 2x input and
  2x output multiplier for every Grok text model that has a long-context tier.
- Add the missing cached-input prices (`grok-4.5` is currently 0.50/MTok where xAI charges 0.30/MTok;
  `grok-4.3`, `grok-build-0.1` and the `grok-4.20-*` family have no cached-input price at all).
- Add the `grok-4.20-0309-reasoning`, `grok-4.20-0309-non-reasoning` and
  `grok-4.20-multi-agent-0309` price entries, which were absent, plus their alias handling.
- Declare the 2x Priority rate for every Grok text model, so priority requests on models other than
  `grok-4.6`/`grok-4.7` are no longer silently billed at standard rates.
- Register `grok-imagine-image-2.0` in the Imagine image price table and teach the tier helper the
  1.5K bucket xAI publishes for the quality model.
- Record the still-unmodelled xAI pricing surfaces (server-side tool invocations, Batch discount,
  US regional endpoint uplift, input-image charges, voice and storage) as known gaps with their
  official values.

## Impact

- Affected specs: `model-catalog`.
- Affected code: both `catalog.json` copies, `backend/resources/model-pricing/model_prices_and_context_window.json`,
  `backend/internal/service/billing_service.go`, `backend/internal/service/image_billing_size.go` (if the
  1.5K bucket needs a new tier).
- No API change and no database migration. Standard short-context pricing for `grok-4.6`/`grok-4.7` and
  the Imagine video prices are already correct and stay unchanged.
- Deploy-gated: the live instance resolves prices from the binary's fallback tables, so the corrected
  rates take effect with the next binary deploy.
