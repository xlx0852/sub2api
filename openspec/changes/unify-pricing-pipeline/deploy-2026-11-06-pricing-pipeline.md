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
