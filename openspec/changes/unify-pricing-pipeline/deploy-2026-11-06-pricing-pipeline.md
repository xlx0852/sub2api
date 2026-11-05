# Deploy 2026-11-06: unified pricing pipeline (slices 1-4)

## Artifacts

- API binary: `0.1.392` (`14614aacc` + version bump `d…`), built `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
- Release dir: `/opt/sub2api/api/releases/0.1.392-pricingpipeline-14614aacc/`
- Frontend: `/opt/sub2api/frontend/releases/0.1.392-14614aacc/` (symlink `current`), previous kept in `rollback`.
- Hot-swapped the bind-mounted binary at
  `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api`, backup
  `sub2api.bak-20261106-pricingpipeline`.

## Migrations applied automatically at startup

- `211_unify_pricing_pipeline.sql`: `price_costs`, `price_modifiers`, and the six new
  `channel_model_pricing` columns.
- `212_seed_pricing_modifiers.sql`: 4 modifier rows (DeepSeek peak ×1.0 ×2, off-peak ×0.5,
  flex ×0.5).
- `213_backfill_price_costs_and_fx_auto.sql`: 8 cost rows, 8 price rows flipped to
  `fx_auto` at `fx_rate=7.0` with the price-equality guard.

## Post-deploy verification

- `channel_model_pricing`: 8 `fx_auto` / 11 `manual`; every price unchanged.
- `price_costs` = 8, `price_modifiers` = 4.
- `/api/v1/public/version` → `0.1.392`; frontend entry chunk hash matches the local build.
- `GET /admin/groups/49/pricing-summary` → each fx_auto row reports `cost_input_usd` and
  `gross_margin_pct = 33.3` (sell ¥ vs cost at ×7 with the 1.5 group multiplier).
- Gateway smoke test on `deepseek-v4.1-flash-abliterated`: HTTP 200, billed
  `0.98 / 3.50` CNY per MTok, identical to pre-deploy (pipeline still `legacy`).
- Temporary probe API key disabled after the test.

## Notes

- `pricing.pipeline` is read from env (`PRICING_PIPELINE`), default `legacy`. Enabling
  `shadow` requires recreating the container with the env set (env is fixed at create),
  or switching the read path to settings first.
- Account #148 (hinoki) was demoted by the scheduled diagnostics chain during the deploy
  window (`gpt-5.4-mini` → 404 `Unknown model`, `hinoki-offensive` → stream deadline).
  That is the pre-existing diagnostics behaviour, unrelated to pricing; it re-enables
  automatically after the demotion window.

## Rollback

- Binary: `cp sub2api.bak-20261106-pricingpipeline sub2api` then restart.
- Frontend: repoint `current` to the `rollback` target.
- Data: all changes are additive; `channel_model_pricing.fx_mode` can be reset to
  `manual` for all rows without touching prices. No `usage_logs` rows were rewritten.

## Follow-up deploys (same day)

| Version | Why | Result |
| --- | --- | --- |
| `0.1.393` | pipeline switch readable from the DB setting `pricing_pipeline` + `GET/PUT /admin/settings/pricing-pipeline` | hot switch, no container recreate |
| `0.1.394` | unset setting returned `404 SETTING_NOT_FOUND` before the first write | GET now reports the effective default |
| `0.1.395` | shadow compared against the pre-discount base price (false diffs at DeepSeek off-peak); invalid mode returned 500 | comparison uses the final legacy price; invalid mode → 400 |

Release dirs: `0.1.393-priceswitch-d44ce087d`, `0.1.394-priceswitch-fix`,
`0.1.395-priceshadow-fix`; the bind-mounted binary is replaced in place each time with
`sub2api.bak-20261106-pricingpipeline` kept as the pre-change rollback.

## Shadow is ON

- `pricing_pipeline` setting = `shadow` (flipped via the admin API at 2026-11-06 00:24 +08).
- Two real requests immediately after the flip (`hinoki-offensive`,
  `deepseek-v4.1-flash-abliterated`) produced **0 `pricing_pipeline.shadow_diff` lines** and
  were billed at exactly the pre-change prices (5.60 / 8.40 and 0.98 / 3.50 CNY per MTok).
- Watch `pricing_pipeline.shadow_diff` for 24h; when it stays empty, flip to `unified` with
  the same endpoint, then remove the legacy constants (`deepSeekOffPeakMultiplier`,
  `serviceTierCostMultiplier`, and the DeepSeek official pin).

## 0.1.396 — admin clarity pass

Deployed binary `0.1.396` (release dir `0.1.396-pricingui-7abb8cf72`) plus frontend
`0.1.396-7abb8cf72` (symlink `current`, previous kept in `rollback`).

What shipped:

- Group list card shows a price example: `模型 售价 → 实付` (实付 = 售价 × 分组倍率),
  e.g. `国模abliterated(rate 1.5) policy=8 | deepseek-v4-1-flash-1017-uncensored 2.31 → 3.465 CNY [fx_auto]`.
- Pricing card locks the manual price inputs for `fx_auto` rows and prints the derivation
  formula from the cost basis (`成本 $x / $y × 汇率 (乘数 k) = 售价 ¥a / ¥b`).
- Models not priced in the policy are flagged as falling back to the built-in default.
- Settings → 定价管线 tab: pipeline switch (legacy/shadow/unified) and the cost-basis table
  with upsert/delete via the new `/admin/settings/price-costs` endpoints.

Verification: `/admin/settings/price-costs` returns the 8 rows; the groups list carries
`example_*`; frontend `index.html` md5 matches the local build; `pricing_pipeline` still
`shadow`.
