## 1. Approval and scope lock

- [x] 1.1 Validate with `openspec validate unify-pricing-pipeline --strict`.
- [ ] 1.2 User approves `proposal.md` + `design.md`; no code lands before that.
- [ ] 1.3 Confirm the only behavior change is structural: existing prices, group multipliers, denomination and peak-window semantics stay identical.

## 2. Schema and data model

- [x] 2.1 Create `price_costs` and add `fx_mode`, `cost_id`, `markup_pct`, `fx_rate`, `context_tiers`, `image_tiers` to `channel_model_pricing`.
- [x] 2.6 Verify the migration applies, is idempotent (run twice) and yields the expected defaults/constraints on a scratch database.
- [x] 2.2 Create `price_modifiers` with scope / target / window / tier / multiplier / enabled / priority.
- [ ] 2.3 Convert `channel_pricing_intervals` rows into `context_tiers` (production currently has 0 rows, so only the conversion code path remains) and verify each model's tier output matches the previous interval output.
- [ ] 2.4 Fold the long-context threshold and multiplier fields into `context_tiers`; keep absolute-price tiers where the official price is absolute.
- [ ] 2.5 Merge the bundled pricing snapshot and the data-dir snapshot into one built-in defaults document in the new schema.

- [x] 2.7 Read the new columns in the channel repository and expose `ListPriceModifiers` / `GetPriceCost` through the port + `ChannelService` (TTL cache, falls back to the previous snapshot if the read fails).
- [x] 2.8 Keep `currency` display-only: charging never reads it, and `fx_mode` decides derivation.

## 3. Derivation (L1 → L2)

- [x] 3.1 Implement `derive()` for `fx_auto` (`cost × (1 − discount) × fx_rate × (1 + markup)`) with a cached per-row result.
- [ ] 3.2 Implement `cny_native` and `manual` passthrough, and assert that `currency` never influences the charge.
- [ ] 3.3 Add the FX rate store (fixed value or daily middle rate) plus the global buffer handled through `markup_pct`.
- [ ] 3.4 Seed `price_costs` for venice (`$0.33/$0.00825/$1.32`), refuseless (list prices, discount field for the active promotions) and hinoki (`$0.80/$0.08/$1.20`).
- [ ] 3.5 Flip exactly the eight live rows to `fx_auto` at `fx_rate=7.0` and assert derived = existing within `1e-9`, otherwise abort.

## 4. Modifier engine (L3)

- [x] 4.1 Implement one `ApplyModifiers` with fixed order vendor_time → request_tier → group_time returning applied modifier ids.
- [x] 4.2 Seed DeepSeek off-peak (`outside`, weekday 01:00–04:00 + 06:00–10:00 UTC, `0.5`) and delete `deepSeekOffPeakMultiplier` / `isDeepSeekPeakTime`.
- [x] 4.3 Seed the `request_tier` flex row (`0.5`) in migration 212 and route it through the engine; `serviceTierCostMultiplier` stays as the legacy path's constant until cutover.
- [ ] 4.4 Route `groups.peak_*` through the same engine without changing its UI or semantics.
- [x] 4.5 Keep the channel-price bypass: `fx_auto` rows are derived first, then vendor_time modifiers apply (a fixed-price row that needs no window simply has no matching modifier).
- [ ] 4.6 Convert the DeepSeek official CNY pins and the GPT-5.6 cache-write rule into defaults data and delete the code paths.

## 5. Image, video and per-request modes

- [ ] 5.1 Move `groups.image_price_1k/2k/4k`, `video_price_480p/720p/1080p` and the batch multipliers into `image_tiers` rows.
- [ ] 5.2 Assert the effective price for every (group, model, tier) triple is unchanged, then drop the group columns in a follow-up release.
- [ ] 5.3 Keep `per_request` mode behaving exactly as today.

## 6. Observability and reporting

- [ ] 6.1 Add `fx_mode`, `fx_rate`, `modifier_ids[]`, `unit_value` to `usage_logs` and populate them on every settlement path.
- [ ] 6.2 Extend the group pricing summary with cost → sell → effective → gross margin.
- [ ] 6.3 Add vendor price drift detection (nightly compare against the last observation, log-only).

## 7. Cutover

- [x] 7.1 Implement `PRICING_PIPELINE=legacy|shadow|unified`: `pricing.pipeline` config, shadow diff logging (`pricing_pipeline.shadow_diff`, one line per model+group), unified charging, and no double off-peak because legacy time-of-day is skipped in unified mode.
- [ ] 7.2 Run shadow in production until the delta log is empty beyond rounding.
- [ ] 7.3 Flip to `unified`; keep `legacy` documented as the one-release rollback.

## 8. Verification

- [ ] 8.1 Unit tests: derivation per mode, tier selection, modifier order, channel-price bypass, fail-closed behaviour.
- [ ] 8.2 Regression: every previously verified price (deepseek CNY official, kimi K3.1 Pro, the eight policy rows, Qwen/MiMo) resolves to the same value.
- [ ] 8.3 Replay a sample of historical usage and assert recalculated charges are byte-identical.
- [ ] 8.4 Run `gofmt`, `git diff --check`, `cmp` on duplicated data files, and the focused unit suites with `CGO_ENABLED=0 -tags unit`.
