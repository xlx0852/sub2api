## 1. Official price verification

- [x] 1.1 Confirm the official model ID and display name on the OpenAI model page (`gpt-6-sol` page
  redirects to `gpt-6.1-sol`); record reasoning-effort support, context/output limits and knowledge
  cutoff.
- [x] 1.2 Capture the official pricing page and record the standard, above-272K, Batch, Flex and Fast
  (priority) rows for `gpt-6.1-sol`.
- [x] 1.3 Diff `gpt-6.1-sol` against `gpt-6-sol` to isolate the real differences (cached-input rate
  5% vs 10%, no `none` reasoning effort, knowledge cutoff) and confirm the short-context input/output
  rates are identical.

## 2. Catalog

- [x] 2.1 Add `gpt-6.1-sol` between `gpt-6-sol` and `gpt-6-luna` in `platforms.openai.models` of the
  canonical catalog, matching the minimal sibling entry shape.
- [x] 2.2 Add the `fallback_pricing["gpt-6.1-sol"]` entry (standard, priority, long-context tier,
  cache breakdown).
- [x] 2.3 Add a `ui_presets.openai` entry labelled "GPT-6.1 Sol".
- [x] 2.4 Repair the alias graph: restore `gpt-5.4`, `gpt-5.4-mini` and `gpt-5.4-nano`
  `fallback_pricing` entries and point `gpt-5.5` at `gpt-5.4` again, so no alias is self-referential
  and the effective fallback prices stay unchanged.
- [x] 2.5 Bump `version` (15 -> 16) and refresh `updated_at`.
- [x] 2.6 Synchronize the embedded copy with `cp` and confirm byte identity.

## 3. Pricing

- [x] 3.1 Add the `gpt-6.1-sol` static fallback (2 USD / 10 USD per MTok, 0.10 USD cache read,
  2.50 USD cache write, 2x priority) in `pricing_service.go`.
- [x] 3.2 Match the `gpt-6.1-sol` prefix ahead of `gpt-6-sol` in `matchOpenAIModel`, leaving the
  existing `gpt-6-sol` branch untouched.
- [x] 3.3 Add the official `gpt-6.1-sol` rows to the pricing mirror (standard, above-272K, batches,
  flex, priority).

## 4. Tests

- [x] 4.1 Catalog test: `gpt-6.1-sol` is present with display name "GPT-6.1 Sol" and
  `use_responses_lite: true`, and `gpt-6-sol` still resolves to its own 0.2 USD cache read.
- [x] 4.2 Pricing service test: static fallback rates for `gpt-6.1-sol` and a date-suffixed variant,
  including the priority and long-context tiers.
- [x] 4.3 Mirror test: default pricing exposes the official GPT-6.1 Sol rates while `gpt-6-sol` keeps
  its own cached-input rate.
- [x] 4.4 Confirm the `modelcatalog` package and the targeted pricing tests pass, and that the service
  package failure set is identical to the clean `HEAD` worktree (no regression).

## 5. Verification

- [x] 5.1 Run `python3 skills/update-model-catalog/scripts/verify_catalog.py` (catalog v16 valid,
  sha256 `694b4ef313cb733b52b30974785ccb317f65bc46157bc239cc275a43973d92b2`).
- [x] 5.2 Run `go build ./...` and the targeted Go tests.
- [x] 5.3 Run `openspec validate update-openai-gpt61-sol-catalog --strict`.
- [x] 5.4 Run `git diff --check`.

## 6. Publication (pending confirmation)

- [x] 6.1 Prepare the validated catalog for the shared repository.
- [ ] 6.2 Copy the catalog, checksum and `versions/16/` snapshot into `xlx0852/model-catalog`, commit
  and push.
- [ ] 6.3 Wait for the `Validate catalog` workflow, verify the published bytes with
  `sha256sum --check`, and confirm the Sub2API refresh status reports version 16 with an empty
  `last_error`.

## Notes

- `supports_none_reasoning_effort: false` follows the official page (`none` reasoning effort is not
  supported on GPT-6.1 Sol); `supports_pdf_input: true` follows the sibling GPT-6 entries. Neither
  key has a code consumer.
- No `above_272k` fields are added to `fallback_pricing`: the `long_context_*` multipliers reproduce
  the official above-272K prices exactly (2.00 x 2 = 4.00 input, 10.00 x 1.5 = 15.00 output).
- `gpt-6.1-sol` is intentionally not added to `openAICodexTurnStateSeedModels` and not added to any
  account model mapping: routing exposure is a separate, account-scoped decision.
- The `normalizeOpenAICodexCompactReasoningEffort` compact downgrade is unchanged; GPT-6.1 Sol keeps
  the same behaviour as GPT-6 Sol.
