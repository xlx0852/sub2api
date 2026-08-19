# DeepSeek mixed-currency cutover runbook

## Atomic application cutover

1. Export a read-only snapshot of active DeepSeek groups, their sales multipliers, bound sell-price policies, API Key accounts, and account `rate_multiplier` values.
2. Record fixed pre-cutover Codex and DeepSeek samples: input/cache/output tokens, `total_cost`, `actual_cost`, and account cost.
3. Deploy the migration and application together. Migration 199 adds currency metadata and removes only the legacy upstream rate-sync flag; it does not update balances, quotas, multipliers, or historical usage amounts.
4. Keep every account `rate_multiplier` operator-controlled. Do not derive or overwrite it from the upstream probe.
5. Verify new DeepSeek pricing snapshots use `CNY`; legacy/missing snapshots display as `USD` without rewriting their amounts.

## Fixed DeepSeek values

- V4 Flash peak per 1M: uncached input `3`, cache read `0.10`, output `9` CNY.
- V4 Pro peak per 1M: uncached input `9`, cache read `0.30`, output `27` CNY.
- Off-peak is 50% of peak.
- Beijing peak windows: `09:00-12:00` and `14:00-18:00`.

For each active group and API Key account, verify:

- user charge = base numeric price x group sales multiplier;
- account cost = base numeric price x account comprehensive cost multiplier;
- gross margin = user charge - account cost.

## Rollback

1. Restore the captured DeepSeek sell-price policy and account multiplier values in the same rollback window.
2. Roll back the application image. Leave the additive currency columns in place; old code ignores them.
3. Do not recalculate or rewrite usage rows written before or during the cutover.
4. Re-run the fixed samples and compare them with the captured pre-cutover values.
