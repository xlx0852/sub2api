## 1. Pricing currency model

- [x] 1.1 Add normalized `USD` / `CNY` currency metadata to fallback catalog pricing, LiteLLM pricing, channel pricing domain/API/persistence, and resolved pricing.
- [x] 1.2 Preserve backward compatibility by treating missing catalog, remote, channel, and historical currency values as `USD`.
- [x] 1.3 Add validation and API contract tests that reject unsupported currencies and prevent one pricing entry from mixing currencies.

## 2. Billing and immutable usage snapshots

- [x] 2.1 Carry resolved pricing currency through the shared billing calculation and provider-neutral settlement input.
- [x] 2.2 Add a nullable `usage_logs.pricing_currency` snapshot and dual-write it through legacy and durable settlement paths without changing historical amount values.
- [x] 2.3 Keep user charge as `base numeric price × sales multiplier` and account cost as `account-side base numeric cost × comprehensive cost multiplier`, with no FX lookup or conversion.
- [x] 2.4 Update usage DTOs, repositories and durable payload compatibility tests for the new snapshot.

## 3. DeepSeek Chinese official pricing

- [x] 3.1 Replace DeepSeek V4 Flash / Pro peak fallback values with the official Chinese CNY prices and retain the existing Beijing peak/off-peak schedule.
- [x] 3.2 Update every bundled model-catalog and model-pricing copy consistently and verify aliases resolve to the same CNY pricing.
- [x] 3.3 Add peak/off-peak tests for cached input, uncached input and output costs for Flash and Pro.
- [x] 3.4 Add regression tests proving Codex/OpenAI USD pricing and existing sales multipliers are unchanged.

## 4. Account comprehensive cost multiplier

- [x] 4.1 Rename administrator-facing account multiplier labels and help text from purchase discount/billing multiplier to comprehensive cost multiplier.
- [x] 4.2 Explain that the operator manually includes exchange rate, upstream discount, fees and risk buffer; do not add an FX field or FX service.
- [x] 4.3 Add a preview showing example account cost and gross margin from the selected pricing basis, sales multiplier and comprehensive cost multiplier.
- [x] 4.4 Update profit/account usage wording while preserving the existing account multiplier fields and historical calculation formula.

## 5. Currency and credit presentation

- [x] 5.1 Display every model base unit price and raw base cost with `$` or `¥` according to pricing currency; default all legacy/missing currency metadata to USD so no model price has an empty unit.
- [x] 5.2 Display user charges, balances, quotas, account costs and profit as plain numbers without currency symbols and without appending credit/cost-credit unit text.
- [x] 5.3 Propagate pricing currency through authenticated and public pricing DTOs without exposing account or private group data.
- [x] 5.4 Add desktop/mobile usage table, pricing catalog, account form and i18n regression tests for USD, CNY and legacy missing-currency rows, including assertions that base prices never render an empty unit and settlement values never append a unit.

## 6. Cutover safety and verification

- [x] 6.1 Produce a read-only DeepSeek cutover report covering active groups, sales multipliers, API Key accounts, comprehensive cost multipliers and sample margins.
- [x] 6.2 Verify no migration changes existing user balances, recharge ratios, API Key quotas or historical usage amounts.
- [x] 6.3 Run targeted backend billing/repository/API tests and frontend unit/type/build checks.
- [x] 6.4 Validate fixed DeepSeek Flash / Pro peak/off-peak samples and unchanged Codex samples before deployment.
- [x] 6.5 Document the atomic DeepSeek pricing-and-multiplier cutover and rollback values for production deployment.
