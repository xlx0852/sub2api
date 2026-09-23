## 1. Fast-tier cache writes

- [x] 1.1 Add `cache_creation_input_token_cost_priority` to `modelcatalog.PriceEntry`.
- [x] 1.2 Map it to `ModelPricing.CacheCreationPricePerTokenPriority` in `priceEntryToModelPricing`.
- [x] 1.3 Record the official Fast-tier cache-write prices for `gpt-6-astra`, `gpt-6-sol`, `gpt-6-luna`,
  `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna` in both catalog copies.

## 2. Tier gating

- [x] 2.1 Remove the blanket `priority => 2.0` fallback in `serviceTierCostMultiplier` so a premium is
  only charged when the model declares priority prices.
- [x] 2.2 Confirm Flex still applies the uniform 0.5x discount.

## 3. Cyber long context

- [x] 3.1 Remove the LiteLLM-sourced `*_above_272k_tokens` and long-context threshold fields from
  `gpt-5.6-cyber` in the pricing mirror (official table shows no long-context tier).

## 4. Upstream-reported tier

- [x] 4.1 Capture the upstream response's `service_tier` for the Responses streaming and non-streaming
  paths (terminal events only on the streaming hot path).
- [x] 4.2 Capture it for the passthrough and raw Chat Completions paths.
- [x] 4.3 Prefer the reported tier over the requested tier in the forward result, and stop the
  chat/messages pipelines from clobbering it with the requested tier.
- [x] 4.4 Take the last SSE frame's value so a `response.created` tier cannot mask the terminal tier.

## 5. Verification

- [x] 5.1 Add `backend/internal/service/billing_service_openai_fast_tier_test.go` covering Fast-tier
  cache writes, un-priced Fast tiers, Flex, and Fast x long-context composition.
- [x] 5.2 Add `backend/internal/service/openai_fast_tier_downgrade_test.go` covering tier extraction
  from JSON/SSE shapes, terminal-only capture, reported-tier preference, and downgraded billing.
- [x] 5.3 Add `backend/internal/pkg/modelcatalog/price_entry_fast_tier_test.go` covering the new price
  entry field and the absence of cyber long-context pricing.
- [x] 5.4 Verify against the official Fast table (gpt-6-sol 4/0.4/5/20) and the Fast mode guide's
  long-context figures for gpt-5.6-sol ($16 in / $60 out per MTok).
- [x] 5.5 Run `openspec validate fix-openai-service-tier-billing --strict`.

## Notes

- The `-tags unit` service test package does not compile at HEAD (pre-existing signature drift in
  `auth_service_register_test.go`, `gateway_account_selection_test.go`, `gateway_group_isolation_test.go`
  and other files), so the new service tests were verified by compiling the package from an explicit
  file list that excludes those pre-existing broken files.
- Compact-bridge responses do not carry `service_tier`, so that path keeps the requested tier.
- The production ops-override catalog installed for the GPT-6 rollout must be deleted when a binary
  carrying these changes is deployed, otherwise it shadows the embedded catalog.

## 6. Production deploy (2026-09-22)

- [x] 6.1 Confirm the live binary's baseline. `go version -m` carries no `vcs.revision`, but the
  running catalog is byte-consistent with commit `0383eff2f` (the only deltas are `PriceEntry` fields
  the admin API cannot re-serialise plus `omitempty` zeroes), and that commit is HEAD's parent.
- [x] 6.2 Build from a clean worktree at HEAD plus this change set only, matching the deployed build
  settings exactly: `-tags=embed -trimpath CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1`,
  unstripped, with `-X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380`.
- [x] 6.3 Canary the binary in a throwaway container on the live network, env and mounts, on port
  18362. Verified health, version, embedded catalog v13, `/v1/models`, a live request, and zero
  `pricing_missing_record_zero_cost` warnings before touching the live container.
- [x] 6.4 Hot-swap the live container: stop, back up the old binary in place, remove the ops-override
  catalog, install the new binary, start. Health returned 200 on the first probe.
- [x] 6.5 Post-deploy verification: embedded catalog v13 active, Fast-tier cache-write prices present
  for the six OpenAI families, Daybreak rows carry no long-context pricing, `/v1/models` exposes
  `gpt-6-sol` and `gpt-6-luna`, and a `service_tier: priority` request bills at the Fast rate
  ($4/MTok input) while a plain request bills the standard $2/MTok.
- [x] 6.6 Remove the canary container and the temporary env file.

### Deploy record

- Deployed binary sha256: `c0f7b5de7da73a741cd8b0cc8004cd6581defc167ed59ecee8a7d9e11abd8165`
- Rollback binary: `/opt/sub2api/api/releases/0.1.391-acct-0383eff2f/sub2api.bak-fasttier-20260922145723`
  (sha256 `76398cb7ed816ad8d3211b0863bebafec9762875aaaf7f8245e1433837edc773`)
- Rollback catalog: `/opt/sub2api/api/releases/0.1.383-external/resources/model-catalog/catalog.json.bak-fasttier-20260922145723`
- Rollback steps: `docker stop sub2api-api-candidate-0.1.391-acct-0383eff2f`, restore the binary backup,
  optionally restore the catalog backup, `docker start`.

### Follow-ups

- `modelcatalog.PriceEntry` still cannot express `*_batches` / `*_flex` prices, so those fields in the
  catalog JSON are dropped on load. Flex is unaffected (it is a uniform 0.5x tier multiplier) and
  `/v1/batch` is not a gateway path, so this is currently latent.
- The live instance still resolves OpenAI prices from the binary's static fallbacks because its
  `PRICING_DATA_DIR` points at an empty read-only mount and the fallback pricing file is absent. That
  configuration predates this change and was left untouched.
