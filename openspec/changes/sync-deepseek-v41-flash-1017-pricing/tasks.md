## 1. Verification

- [x] 1.1 Confirm the prod price table and channel pricing contain no entry for `deepseek-v4.1-flash-1017` (only `deepseek-v4-flash`, `deepseek-v4-flash-vision-exp`, `deepseek-v4-pro`, `deepseek-v4.1-flash`).
- [x] 1.2 Reproduce the gap: `getFallbackPricing` returns nil for `deepseek-v4-1-flash-1017-uncensored`, `deepseek-v4.1-flash-1017-uncensored`, `deepseek-v4.1-flash-1017`, and `deepseek-v4.1-flash-hc`, while `deepseek-v4-flash-0731` matched.
- [x] 1.3 Confirm the official V4.1 Flash CNY rates (2.00 / 0.04 / 2.00 / 8.00 per 1M, half off-peak) from the DeepSeek pricing page and from the existing `deepseek-v4.1-flash` entry.
- [x] 1.4 Confirm the zero-cost precedent in `usage_logs` (`deepseek-v4.1-flash-hc`, 2026-10-02, `total_cost = 0`).

## 2. Catalog and pricing data

- [x] 2.1 Add `deepseek-v4.1-flash-1017` to `fallback_pricing` with `currency: CNY` and the official V4.1 Flash rates.
- [x] 2.2 Bump the catalog version to `18` with a fresh `updated_at`.
- [x] 2.3 Keep `backend/internal/pkg/modelcatalog/catalog.json` and `backend/resources/model-catalog/catalog.json` byte-identical.
- [x] 2.4 Mirror the entry in `backend/resources/model-pricing/model_prices_and_context_window.json` next to `deepseek-v4.1-flash`.

## 3. Billing resolution

- [x] 3.1 Match the dotted `deepseek-v4.1-flash` family by prefix in `isDeepSeekV4OfficialModel` so suffixed spellings get the official peak pin and the off-peak half-price window.
- [x] 3.2 Leave the existing exact-match branch, the dash-form `deepseek-v4-flash` branch, the Pro branch, and the chat/reasoner aliases untouched.
- [x] 3.3 Do not price other vendor suffixes (`-hc`, `-uncensored`); they must keep failing closed until priced deliberately.

## 4. Regression tests

- [x] 4.1 Assert `deepseek-v4.1-flash-1017` resolves the CNY entry through fallback family matching.
- [x] 4.2 Assert peak input/cache-write/cache-hit/output costs of 2 / 2 / 0.04 / 8 CNY and exact off-peak halving.
- [x] 4.3 Add `deepseek-v4.1-flash` itself to the same exact-CNY component test.
- [x] 4.4 Update the stale DeepSeek V4 Flash expectations (`3 / 9 / 0.10`) in the fallback-matching test to the current CNY rates.

## 5. Verification

- [x] 5.1 Run the focused service and catalog unit tests with `CGO_ENABLED=0 -tags unit`.
- [x] 5.2 Run `gofmt`, `git diff --check`, and `cmp` on both catalog copies.
- [x] 5.3 Validate this change with `openspec validate sync-deepseek-v41-flash-1017-pricing --strict`.

## 6. Deployment (pending)

- [ ] 6.1 Rebuild the API binary so the embedded catalog and the predicate change ship together.
- [ ] 6.2 Hot-swap the binary in the running container (`docker stop` → replace → `docker start`).
- [ ] 6.3 Verify with one priced request or a pricing probe: `deepseek-v4.1-flash-1017` settles at CNY 2 / 0.04 / 8 (peak) instead of 0 or 503.
