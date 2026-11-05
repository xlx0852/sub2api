## 1. Static fallback pricing layer

- [ ] 1.1 Add `openAIGPT55FallbackPricing` in `backend/internal/service/pricing_service.go`:
  input 5e-6, output 3e-5, cache read 5e-7, priority input 1.25e-5, priority output 7.5e-5,
  priority cache read 1.25e-6, long context 272000 / 2.0 / 1.5, `SupportsServiceTier` true, and no
  cache-creation price.
- [ ] 1.2 Add `openAIGPT55ProFallbackPricing`: input 3e-5, output 1.8e-4, cache read 3e-6,
  long context 272000 / 2.0 / 1.5, `SupportsServiceTier` true, and no priority or cache-creation price.
- [ ] 1.3 Insert a `gpt-5.5-pro` prefix branch ahead of the `gpt-5.5` prefix branch and repoint the
  `gpt-5.5` branch from `openAIGPT54FallbackPricing` to `openAIGPT55FallbackPricing`, keeping the
  existing log line shape with the new target label.
- [ ] 1.4 Leave `gpt-5.4*` branches, `serviceTierCostMultiplier`, and the
  `normalizeKnownOpenAICodexModel` family matching untouched.

## 2. Catalog fallback layer

- [ ] 2.1 Replace the `gpt-5.5` and `gpt-5.5-pro` alias entries in
  `backend/resources/model-catalog/catalog.json` with self-contained price entries matching task 1.1/1.2
  and omitting `cache_creation_input_token_cost` and `supports_cache_breakdown`.
- [ ] 2.2 Bump `version` 16 -> 17 and set `updated_at` to the release-time UTC RFC 3339 timestamp.
- [ ] 2.3 Copy the canonical file over `backend/internal/pkg/modelcatalog/catalog.json` and confirm the
  two copies are byte-identical (`skills/update-model-catalog/scripts/verify_catalog.py`).
- [ ] 2.4 Confirm no other `fallback_pricing` entry changed and that the alias graph stays cycle-free.

## 3. Pending-change conflict correction

- [ ] 3.1 In `openspec/changes/update-openai-gpt61-sol-catalog/specs/model-catalog/spec.md`, replace the
  `gpt-5.5`-through-`gpt-5.4` alias example with a still-valid chain (`claude-opus-4.8` ->
  `claude-opus-4.5`).
- [ ] 3.2 Drop the `gpt-5.5-pro` clause from the "Restored GPT-5.4 fallback entries keep effective
  prices" scenario, keeping the `gpt-5.4-mini` assertion.

## 4. Tests

- [ ] 4.1 Add a service-layer test covering `gpt-5.5` standard rates (5e-6 / 5e-7 / 3e-5), the Fast tier
  (1.25e-5 / 1.25e-6 / 7.5e-5 applied only for `service_tier=priority`), and the 272K long-context
  multipliers.
- [ ] 4.2 Add coverage that `gpt-5.5-pro` resolves to its own constant (3e-5 / 3e-6 / 1.8e-4) and stays
  at standard rates when billed on the priority tier.
- [ ] 4.3 Add coverage that date-suffixed and effort-suffixed variants (`gpt-5.5-2026-04-23`,
  `gpt-5.5-medium`) inherit the family price, and that with no pricing data loaded the static fallback
  returns the official rates instead of the GPT-5.4 rates.
- [ ] 4.4 Add a catalog test asserting both GPT-5.5 entries are self-contained (no `alias_of`), carry no
  cache-creation price, and declare the expected priority prices for `gpt-5.5` only.
- [ ] 4.5 Run `go test ./internal/service/... ./internal/pkg/modelcatalog/...` (no `-race` on this host).
- [ ] 4.6 Run `openspec validate update-openai-gpt55-fallback-pricing --strict`.

## 5. Release (separate approval required)

- [ ] 5.1 Build the frontend (pnpm) and the binary with `-tags embed` after the catalog bump.
- [ ] 5.2 Deploy by hot-swapping the release binary, then verify production logs show
  `gpt-5.5 -> gpt-5.5(static)` and no `gpt-5.5 -> gpt-5.4(static)` for new traffic.
- [ ] 5.3 Verify a sampled `gpt-5.5` settlement matches the official rates (input 2x, cache read 2x,
  output 2x of the previous charge).
- [ ] 5.4 Publish catalog v17 to the shared `xlx0852/model-catalog` repository once, with the matching
  `catalog.sha256` and `versions/17/catalog.json`.
- [ ] 5.5 Archive this change under `openspec/changes/archive/` in a separate PR (Stage 3).
