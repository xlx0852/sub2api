## 1. Pricing configuration
- [x] 1.1 Add official CNY pricing entries for the 5 Qwen and MiMo models to `catalog.json` (in `internal/pkg/modelcatalog` and `resources/model-catalog`).
- [x] 1.2 Add pricing metadata to `model_prices_and_context_window.json` and `backend/data/model_pricing.json`.
- [x] 1.3 Update `BillingService.getFallbackPricing` to resolve exact matches for the 5 models in CNY.

## 2. Verification and tests
- [x] 2.1 Add unit tests in `billing_service_test.go` confirming exact pricing resolution and cost calculation.
- [x] 2.2 Verify unknown aliases (`qwen-max`, `mimo`) continue to fail closed with `ErrModelPricingUnavailable`.
- [x] 2.3 Run full test suite for `modelcatalog` and pricing resolution verification.
