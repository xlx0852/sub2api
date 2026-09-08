# Change: Add Qwen and MiMo Model Pricing

## Why

The platform currently lacks pricing configurations for Alibaba Qwen (`qwen3.7-max`, `qwen3.8-flash`, `qwen3.8-max`) and Xiaomi MiMo (`mimo-v2.5`, `mimo-v2.5-pro`) models. Calls to these models without channel-specific pricing overrides fail with `ErrModelPricingUnavailable`.

## What Changes

- Add exact official baseline CNY pricing entries for `qwen3.7-max`, `qwen3.8-flash`, `qwen3.8-max`, `mimo-v2.5`, and `mimo-v2.5-pro` to the fallback model catalog (`catalog.json`) and dynamic pricing files (`model_pricing.json`, `model_prices_and_context_window.json`).
- Update `BillingService.getFallbackPricing` to support exact matching for these 5 models while preserving strict whitelist semantics (no implicit broad fallback for unknown variants like `qwen-max` or `mimo`).
- Add regression tests verifying token billing calculation and pricing resolution for all 5 models.

## Impact

- Affected specs: `model-catalog`
- Affected code:
  - `backend/internal/pkg/modelcatalog/catalog.json`
  - `backend/resources/model-catalog/catalog.json`
  - `backend/resources/model-pricing/model_prices_and_context_window.json`
  - `backend/data/model_pricing.json`
  - `backend/internal/service/billing_service.go`
  - `backend/internal/service/billing_service_test.go`
- No database migrations required.
