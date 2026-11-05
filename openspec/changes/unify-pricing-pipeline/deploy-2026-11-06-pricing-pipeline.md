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

## 0.1.397 — standalone pricing page (unit price has one editing surface)

- `GET /admin/sell-price-policies/overview` (new): every table with rows per MTok, bound
  groups + their discounts, `priced_row_count` (0 = model-list container), and one example
  (sell → effective). Priced+bound tables sort first.
- New page `/admin/pricing` with tabs 售价表 / 按模型价目 (matrix shows every offer per model
  and flags diverged models). Legacy `/admin/sell-price-policies` and `/admin/channels`
  redirect there; sidebar has a Pricing entry.
- The pricing editor gained a policy mode (edit a table without a group); the group modal
  keeps editing but links to the pricing page.

Live overview right after deploy (this is the first time the divergence is visible):

```
#4  GPT Pro        11 models / 1 priced   bound: GPT Pro ×0.18
#8  国模无 s        9 models / 8 priced   bound: 国模abliterated ×1.5
#5  Grok Heavy     12 models / 0 priced   bound: Grok Heavy ×0.16   ← 只有模型清单，没有价
#7  Pro 外接       10 models / 1 priced   unbound                  ← 绑定分组已被删除
#3  企业高可用      10 models / 1 priced   unbound                  ← 绑定分组已被删除
#6  Claude Max20    9 models / 0 priced   unbound                  ← 绑定分组已被删除
```

## Sell-table consolidation (2026-11-06 ~01:00 +08)

Deleted four tables via the admin API, keeping the two that actually price something:

| Deleted | Why |
| --- | --- |
| `#3 企业高可用 售价` | only row duplicated the luna price, bound group (29) already deleted |
| `#7 Pro 外接 售价` | same luna duplicate, bound group (25) already deleted |
| `#5 Grok Heavy 售价` | 12 model names, **zero prices** (pure model list), bound to a live group |
| `#6 Claude Max20 售价` | zero prices, unbound |

Result: `#4 GPT Pro 售价` (luna) and `#8 国模无 s 售价` (8 cost-derived rows) remain;
`Grok Heavy` (23) and `国模集合` (34) are unbound and resolve through the built-in defaults.

Verification before/after (pricing-summary diff per group, values rounded to 1e-9):

- group 4: 11 models, 0 differences
- group 49: 9 models, 0 differences
- groups 23/34: no priced rows on either side; grok models still resolve through defaults
  (`grok-4.3` → $1.25/$2.50 per MTok, `grok-build-0.1` → $1.00/$2.00)
- live probe on group 49 (`hinoki-offensive`): HTTP 200, billed 5.60 / 8.40 CNY per MTok —
  identical to pre-merge. (The two 404s in between were account #148 being temporarily
  demoted by scheduled diagnostics, not the merge; it recovered at 01:02 and the probe passed.)

Backups taken before the change: `/tmp/backup-channels.csv`, `/tmp/backup-pricing.csv`,
`/tmp/backup-bindings.csv` on the prod host.

### Why the two remaining tables were not merged into one

Merging `#8` into a single table shared with `GPT Pro` would publish the uncensored models
at `price × 0.18` (e.g. hinoki ¥5.60 → ¥1.01 per MTok) against a ¥5.74 cost — an 82% loss
for any group whose account pool can serve those model ids. One table only works while every
group's difference is proportional; 1.5 (markup) and 0.16/0.18 (reseller discounts) are not
compatible regimes for the same rows. Options if a single table is still wanted: restrict
model visibility per group and rely on admission, raise the shared row prices so `×0.18`
lands at cost, or accept the risk knowingly.

## 0.1.398 / 0.1.399 — retire the OpenAI `/v1/messages` dispatch capability

Usage check first: `/v1/messages` on the only OpenAI group with the flag enabled (GPT Pro)
had **10 requests in 30 days**; group #49 carried a leftover config with the flag off.
`/v1/messages` traffic on groups 23 (grok) and 34 (anthropic) is unrelated to this feature.

Removed:

- backend: the Claude-family → GPT mapping (`Opus/Sonnet/Haiku` + exact overrides), the
  allow flag gate (OpenAI groups now always 403), and the admin request/response fields
  (`allow_messages_dispatch`, `messages_dispatch_model_config`, `default_mapped_model`).
  grok / kimi native `/v1/messages` support is untouched.
- frontend: the whole "OpenAI Messages 调度配置" block (create + edit dialogs), the helper
  module and its spec, the "Claude Code" tab in the API-key usage modal, i18n keys.
- migration 214 clears the three columns on every group (columns kept, repository writes
  zeros; dropping them would need an ent regeneration).

Live verification (0.1.399):

```
POST /v1/messages (group 4)                 → 403 {"message":"This group does not support /v1/messages (use an Anthropic group)"}
POST /v1/messages/count_tokens (group 4)    → 403 same message
GET  /admin/groups/4                        → response no longer carries any dispatch field
groups table                                → allow_messages_dispatch=f, config={}, default_mapped_model=''
ops_error_logs                              → only the expected two 403s from the probes
```
