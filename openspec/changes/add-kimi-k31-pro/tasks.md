## 1. Proposal and official verification

- [x] 1.1 Confirm the canonical API ID `kimi-k3.1-pro` and display name `Kimi K3.1 Pro` from the official quickstart.
- [x] 1.2 Confirm the 1,048,576-token context window, reasoning, streaming, image/video input, and tool-calling capabilities.
- [x] 1.3 Confirm the standard and Highspeed CNY rates from the official chat pricing page.
- [x] 1.4 Define the three compatibility spellings and keep catalog, price, and upstream normalization behavior separate.
- [x] 1.5 Validate this change with `openspec validate add-kimi-k31-pro --strict`.

## 2. Model catalog

- [x] 2.1 Add the `kimi-k3.1-pro` model entry with reasoning, image/video, and 1,048,576-token context metadata.
- [x] 2.2 Add the self mapping, `kimi3.1` compatibility alias, and `Kimi K3.1 Pro` UI preset.
- [x] 2.3 Keep the existing `k3`, `k3-256k`, K2.7 Code, and K2.7 Code HighSpeed entries unchanged.
- [x] 2.4 Bump the catalog version and timestamp once, then keep both `catalog.json` copies byte-identical.

## 3. Pricing

- [x] 3.1 Add only `kimi-k3.1-pro` to `fallback_pricing`, using CNY and the confirmed standard rates.
- [x] 3.2 Populate the four existing priority fields with the confirmed Highspeed rates, leaving cache-hit pricing at `1.30e-6`.
- [x] 3.3 Resolve all three compatibility spellings through the exact K3.1 Pro branch before the broad `kimi-k3` fallback.
- [x] 3.4 Add the canonical price to the local model-pricing snapshot without replacing or renumbering unrelated entries.
- [x] 3.5 Leave `computeTokenBreakdown` and the billing core unchanged.

## 4. Kimi upstream normalization

- [x] 4.1 Normalize `kimi3.1`, `kimi-k3.1-pro`, and `moonshot/kimi-k3.1-pro` to `kimi-k3.1-pro`.
- [x] 4.2 Preserve trailing `(...)` suffixes and continue removing a trailing `[1m]` marker before mapping.
- [x] 4.3 Leave all existing Kimi K3 and K2.7 model mappings unchanged.

## 5. Regression tests

- [x] 5.1 Extend catalog tests for the Kimi model list, mapping, alias, preset, and self-mapping.
- [x] 5.2 Extend normalization tests for all three K3.1 Pro spellings, `[1m]`, and `(...)` suffix preservation.
- [x] 5.3 Assert canonical and compatibility spellings resolve the CNY standard input, cache-hit, cache-write, and output prices.
- [x] 5.4 Assert `service_tier=priority` selects the Highspeed input, cache-hit, cache-write, and output prices.
- [x] 5.5 Assert `kimi-k3.1-pro` resolves its exact entry rather than the legacy broad `kimi-k3` fallback.

## 6. Verification

- [x] 6.1 Validate both catalog files and run the focused model-catalog and service unit tests with `CGO_ENABLED=0`.
- [x] 6.2 Run `gofmt` on changed Go files, `git diff --check`, `cmp` on both catalog copies, and inspect `git status`.
- [x] 6.3 Confirm no unrelated GPT, Grok, scheduler-cache, or tooling changes are included in this change.
