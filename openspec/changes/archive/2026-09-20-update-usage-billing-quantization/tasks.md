## 1. Quantization contract

- [x] 1.1 Implement a decimal-safe positive charge ceiling at six fractional digits
- [x] 1.2 Cover zero, exact-scale, below-scale, boundary, large-value, NaN and infinity behavior

## 2. Billing integration

- [x] 2.1 Apply the canonical charge to the shared gateway billing path
- [x] 2.2 Apply the canonical charge to the OpenAI-specific usage path
- [x] 2.3 Reuse the customer charge for balance, subscription, API Key quota/rate limit, platform quota and billing caches; quantize account-cost quota independently
- [x] 2.4 Persist the same canonical charge as `usage_logs.actual_cost` while retaining raw component costs
- [x] 2.5 Keep request fingerprints derived from raw pre-quantization amounts

## 3. Verification

- [x] 3.1 Add exact cross-ledger reconciliation and retry-idempotency regression tests
- [x] 3.2 Run targeted billing, gateway usage and repository tests
- [x] 3.3 Run a read-only recent-production delta simulation and document the maximum and observed revenue effect
