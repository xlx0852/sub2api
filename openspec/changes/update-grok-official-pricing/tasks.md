## 1. Grok text rate card

- [x] 1.1 Set the long-context threshold to 200,000 tokens with a 2x input and 2x output multiplier for
  `grok-4.5`, `grok-4.3`, `grok-build-0.1` and the three `grok-4.20-*` models.
- [x] 1.2 Correct cached-input rates: `grok-4.5` 0.30 USD/MTok (was 0.50), and add the missing cached
  rates for `grok-4.3`, `grok-build-0.1` and `grok-4.20-*` at 0.20 USD/MTok.
- [x] 1.3 Add the `grok-4.20-0309-reasoning`, `grok-4.20-0309-non-reasoning` and
  `grok-4.20-multi-agent-0309` entries, which were absent from the catalogue.
- [x] 1.4 Add `alias_of` entries for the `grok-4.20-reasoning` and `grok-4.20-non-reasoning` spellings.
- [x] 1.5 Declare the 2x Priority rates for every Grok text model.
- [x] 1.6 Align the pricing mirror: add the missing 200k long-context threshold to `grok-4.6`/`grok-4.7`
  and add the `grok-4.5`, `grok-4.3`, `grok-build-0.1` and `grok-4.20-*` rows.

## 2. Grok Imagine images

- [x] 2.1 Register `grok-imagine-image-2.0` in the Grok Imagine image price table with the model's
  default (auto/medium) quality rates: 1K = 0.06, 2K/4K = 0.08 USD per image.
- [x] 2.2 Add the model to the Grok catalogue list and default mapping, and to the pricing mirror.

## 3. Verification

- [x] 3.1 Add `backend/internal/service/grok_official_pricing_test.go` covering per-model short-context
  rates, Priority = 2x standard, whole-request long-context pricing, the exact 200k boundary, and the
  Imagine image and video rates.
- [x] 3.2 Run `openspec validate update-grok-official-pricing --strict`.

## 4. Deferred surfaces (recorded, not implemented)

These xAI pricing surfaces are published but not modelled by the gateway. They are listed so coverage
is not overstated.

- [ ] 4.1 **Server-side tool invocations** — `web_search` 5 USD / 1k calls; `x_search` 5 USD / 1k posts
  plus 10 USD / 1k profiles; `code_execution`/`code_interpreter` 5 USD / 1k calls; `attachment_search`
  10 USD / 1k calls; `collections_search`/`file_search` 2.50 USD / 1k calls; `image_generation` at
  Imagine rates; `view_image`, `view_x_video` and remote MCP tools are token-billed (already covered).
  Requires counting server-side tool items on the response path plus a usage-log field and migration.
  The same gap exists for OpenAI web search at 10 USD / 1k calls.
- [ ] 4.2 **Batch discount** — 20% for `grok-4.3`, `grok-build-0.1`, `grok-4.20-0309-reasoning`,
  `grok-4.20-0309-non-reasoning` and `grok-4.20-multi-agent-0309`; no discount for other Grok models.
  `/v1/batch` is not a gateway ingress path today.
- [ ] 4.3 **US regional endpoint uplift** — `https://us.api.x.ai/v1` bills 1.1x the global rates on
  input, cached input and output, including long-context rates. `xai/oauth.go` accepts `*.api.x.ai`
  regional hosts, so an account pointed at a regional endpoint would currently be under-billed. No
  account uses one today (all four OAuth accounts use `https://api.x.ai/v1`).
- [ ] 4.4 **Input media charges** — `pricePerInputImage` 0.01 USD for `grok-imagine-image-2.0` and
  `grok-imagine-image-quality`, 0.002 USD for `grok-imagine-image`; `grok-imagine-video` input video
  0.01 USD/sec. Image/video input counts are not carried into image billing.
- [ ] 4.5 **Voice and storage** — speech-to-speech `grok-voice-think-fast-2.0` 0.08 USD/min;
  speech-to-text 0.10 USD/hr REST and 0.20 USD/hr streaming; text-to-speech 15 USD / 1M characters;
  file storage 0.025 USD/GiB/day, collection storage 0.10 USD/GiB/day, downloads 0.20 USD/GiB.
- [ ] 4.6 **Usage guideline violation fee** — 0.05 USD per Responses request rejected before
  generation.

## Notes

- The Imagine image price table has no quality dimension, so `grok-imagine-image-2.0` is priced at the
  model's default (auto/medium) quality. Adding low/medium quality tiers requires carrying the request's
  `quality` parameter into image billing.
- Image billing tiers are 1K/2K/4K derived from pixel dimensions, so xAI's 1.5K bucket folds into 2K.
  For `grok-imagine-image-quality` that means a 1.5K image bills 0.07 instead of 0.06. Introducing a real
  1.5K tier would change classification for every image model, so it was left alone.
- `grok-composer-2.5-fast` has no public xAI rate card entry (it is Cursor-owned); its existing entry is
  unchanged and unverified against xAI.
- Deploy-gated: the live instance resolves prices from the binary's fallback tables, so these corrected
  rates take effect with the next binary deploy.

## 5. Production deploy (2026-09-22)

- [x] 5.1 Build from a clean worktree at HEAD plus the accumulated change set, matching the live build
  settings exactly: `-tags=embed -trimpath CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1`,
  unstripped, with `-X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380`.
- [x] 5.2 Canary the binary in a throwaway container on the live network, env and mounts, on port
  18362. Verified health, version, embedded catalogue v15 with the corrected Grok rates, the
  `grok-imagine-image-2.0` registration, and live requests for `grok-4.7`, `grok-4.5` and
  `grok-build-0.1` before touching the live container.
- [x] 5.3 Hot-swap the live container with an in-place binary backup and an automatic rollback guard.
  Health returned 200 on the first probe.
- [x] 5.4 Post-deploy verification: catalogue v15 active, live Grok requests return 200 and bill at
  $2/MTok (`grok-4.7`, `grok-4.5`) and $1/MTok (`grok-build-0.1`), zero panics, zero
  `pricing_missing_record_zero_cost` warnings, and the `GK` provider still answers for `grok-4.7`
  and `grok-4.6`.
- [x] 5.5 Remove the canary container and its temporary env file.

### Deploy record

- Deployed binary sha256: `03c524691bf555086ff115f67ea27b58bac75d05b52c121cf3d077ec78381a81`
- Rollback binary: `/opt/sub2api/api/releases/0.1.391-acct-0383eff2f/sub2api.bak-grokpricing-20260922151055`
  (sha256 `c0f7b5de7da73a741cd8b0cc8004cd6581defc167ed59ecee8a7d9e11abd8165`)
- Rollback steps: `docker stop sub2api-api-candidate-0.1.391-acct-0383eff2f`, restore the binary
  backup, `docker start`.

### Verification limits

- The 200k long-context tier and the corrected cached-input rate were not exercised against the live
  upstream: a 200k-token request costs real money and cache-hit traffic is not deterministic. They are
  covered by unit tests over the same resolution path the gateway uses.
- A live `service_tier: priority` request was served at the default tier by xAI, so it billed standard
  rates — the documented xAI behaviour and the deployed upstream-tier rule. The 2x Priority rates
  themselves are covered by unit tests.
