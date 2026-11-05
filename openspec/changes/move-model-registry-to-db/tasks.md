## 1. Approval and baseline

- [ ] 1.1 `openspec validate move-model-registry-to-db --strict`.
- [ ] 1.2 User approves proposal + design. Slice 1 below may ship on its own.
- [ ] 1.3 Capture a baseline: per-group resolved price for every priced model (`pricing-summary` dump) to diff against after each slice.

## 2. Slice 1 — additive schema and seed (no behaviour change)

- [x] 2.1 Migration 215: create `models`, `model_aliases`, `model_prices` with the constraints from the design (including the one-default-per-model partial index).
- [x] 2.2 Migration 216: seed from both `catalog.json` copies — models, aliases (`aliases` / `id_overrides` / `id_reverse_overrides` / `retired_ids`), default chat/test flags, and default prices from `fallback_pricing`.
- [x] 2.3 Migration 216: import the live sell rows (2 tables, 18 model names) into `model_prices` as per-group rows, preserving `fx_mode`, `cost_id`, `fx_rate`, `markup_pct`.
- [x] 2.4 Post-migration assertions: every imported price row resolves to the same value as `channel_model_pricing`, and no row has four empty token prices.
- [x] 2.5 Verify the migration is idempotent and aborts atomically on assertion failure (scratch-database run with the real rows, like migration 213).

- [x] 2.6 Seed generator (`scripts/gen_model_registry_seed.py`) keeps the SQL in sync with catalog.json instead of hand-writing 120+ inserts.
- [x] 2.7 Scratch-database run with the real 13 price rows / 46 groups / 8 cost rows: 123 models, 10 aliases, 69 default prices, 10 group prices; second run idempotent; tampering with one group price makes the assertion abort; a row with no price is rejected.

## 3. Slice 2 — loaders and dual read

- [ ] 3.1 `modelcatalog.Load` accepts a database source and builds the same `Catalog` struct; `MODEL_REGISTRY_SOURCE=file|dual|db` via settings, hot switch with short TTL.
- [ ] 3.2 `dual` mode compares model sets, aliases, default chat/test models and retired ids against the file, logging one line per difference.
- [ ] 3.3 `ModelPricingResolver` gains a `model_prices` source with order group row → default row → fail closed; `MODEL_PRICING_SOURCE=legacy|dual|model_prices`.
- [ ] 3.4 `dual` mode logs `pricing_source.mismatch` with model, group, both prices; charging stays legacy.
- [ ] 3.5 Registry alias resolution runs before pricing (canonical id used for both price lookup and upstream mapping).

## 4. Slice 3 — guards and admin surface

- [ ] 4.1 Reject price rows for unregistered models (`MODEL_NOT_REGISTERED`) and rows with no price at all.
- [ ] 4.2 Alias conflicts (same alias, different model) rejected; retired ids cannot be re-registered as active.
- [ ] 4.3 `/admin/models` page: list with defaults column and red flag for `MISSING_DEFAULT_PRICE`; detail drawer for identity, aliases and per-group prices (default row first).
- [ ] 4.4 `/admin/pricing` becomes a read-only comparison view (matrix, divergence, effective preview) and links to the model page for edits.
- [ ] 4.5 "Register from vendor" action: pull `/v1/models` from an account, prefill id/context/capabilities/price, require human confirmation.
- [ ] 4.6 Optimistic locking (`version`) on save with a clear conflict error.

## 5. Cutover

- [ ] 5.1 Run `MODEL_REGISTRY_SOURCE=dual` + `MODEL_PRICING_SOURCE=dual` in production until the mismatch logs are empty for 24 h.
- [ ] 5.2 Switch to `db` / `model_prices`; verify with a live request per platform.
- [ ] 5.3 Keep `channel_model_pricing` readable for one release; document the rollback switch pair.

## 6. Verification

- [ ] 6.1 Unit tests: resolution order, alias canonicalisation, priority prices, empty-row rejection, unregistered rejection.
- [ ] 6.2 Regression: seeded defaults equal the previous resolved prices for every model/group (baseline diff from 1.3).
- [ ] 6.3 Frontend: `vue-tsc`, model page component test, pricing page read-only test.
- [ ] 6.4 `gofmt`, `go build ./...`, focused `-tags unit` suites, migration idempotency run.
