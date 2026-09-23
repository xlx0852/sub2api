## 1. Sidecar aliases

- [x] 1.1 Add `oauth-model-alias` entries for the nine GPT-family Devin models, aliasing each to our
  standard id (`devin/gpt-5-5` → `gpt-5.5`, `devin/gpt-5-4` → `gpt-5.4`, `devin/gpt-5-4-mini` →
  `gpt-5.4-mini`, `devin/gpt-5-3-codex` → `gpt-5.3-codex`, `devin/gpt-4-1` → `gpt-4.1`,
  `devin/gpt-6-astra` → `gpt-6-astra`, `devin/gpt-5-6-sol` → `gpt-5.6-sol`, `devin/gpt-5-6-luna` →
  `gpt-5.6-luna`, `devin/gpt-5-6-terra` → `gpt-5.6-terra`) with `force-mapping: true`.
- [x] 1.2 Restart the sidecar and confirm the catalog publishes the standard ids and that each one
  answers on `/v1/chat/completions`.

## 2. Account mapping and priority

- [x] 2.1 Add the nine standard ids to the Devin account's `model_mapping` as identity entries, keeping
  the existing `swe` entries, model, groups and status untouched.
- [x] 2.2 Move the account into the lowest scheduling priority tier so the existing accounts keep
  serving those ids first.

## 3. Verification

- [x] 3.1 End-to-end through the public gateway on a route that reaches only the Devin account: a
  `gpt-5.5` request returns 200, the client-visible model is `gpt-5.5` (streaming and non-streaming),
  and a tool call round-trips.
- [x] 3.2 The usage row keeps the standard id, is priced (non-zero cost for a non-trivial request), and
  its account is the Devin account.
- [x] 3.3 Confirm the existing pools are undisturbed: the Devin account sits in the lowest priority tier
  and the pre-existing account priorities in the shared group are unchanged.
- [x] 3.4 `openspec validate add-devin-gpt-model-translation --strict`.

## 4. Documentation

- [x] 4.1 Record the id translation table, the priority decision, and how to promote Devin from overflow
  to peer (or expose the remaining ids) in the deploy record.

### Deploy record

- Sidecar: `oauth-model-alias` block extended on
  `/opt/sub2api/bridges/devin-cliproxy/releases/v7.3.13/config.yaml` (backup kept as
  `config.yaml.bak-pre-gpt-alias`), container restarted. The catalog now publishes the nine standard ids
  in place of the `devin/…` names for those entries (still 40 models).
- Account 126: `credentials.model_mapping` gained the nine identity entries (3 `swe` + 9 standard =
  12 entries), and `priority` moved `1 → 50` (the repository default tier). Base URL, API key, platform,
  bridge kind, `credentials.model`, status and group membership are unchanged.
- Id translation table (sidecar alias → client-visible id):
  `devin/gpt-5-5`→`gpt-5.5`, `devin/gpt-5-4`→`gpt-5.4`, `devin/gpt-5-4-mini`→`gpt-5.4-mini`,
  `devin/gpt-5-3-codex`→`gpt-5.3-codex`, `devin/gpt-4-1`→`gpt-4.1`, `devin/gpt-6-astra`→`gpt-6-astra`,
  `devin/gpt-5-6-sol`→`gpt-5.6-sol`, `devin/gpt-5-6-luna`→`gpt-5.6-luna`,
  `devin/gpt-5-6-terra`→`gpt-5.6-terra`.

### Verification

- Sidecar direct: all nine standard ids answer `200` on `/v1/chat/completions` and echo the standard id
  back; the catalog lists them.
- Account path through the gateway: `POST /api/v1/admin/accounts/126/test` with `model_id=gpt-5.5`
  streams a completion whose `model` is `gpt-5.5`; `model_id=swe-2` still works.
- Public path (temporary canary group bound to account 126 only, removed afterwards):
  `gpt-5.5` answered `200` non-streaming (`model=gpt-5.5`, content `ok`, 395 tokens) and streaming, and a
  tool call returned `get_weather {"city":"Paris"}`.
- Billing: the canary requests landed as `account_id=126, model=gpt-5.5` with non-zero cost
  (0.00105 / 0.001333) — the standard id hits the existing price entry, unlike the namespaced
  `devin/gpt-*` ids, which have no price and would record zero.
- No diversion: a normal group-4 `gpt-5.5` request was served by account 120 (higher tier), not by
  Devin, confirming the lowest-tier placement.
- Priorities in the shared group are unchanged (`69=10`, `89=0`, `120=0`, Devin `50`).
- The temporary canary group is soft-deleted, with no keys or accounts attached, and absent from the
  admin group list; the rebound test key was restored to group 4.

### Follow-ups

- Six of the nine ids (`gpt-5.3-codex`, `gpt-4.1`, `gpt-6-astra`, `gpt-5.6-*`) are mapped on the account
  but absent from every group's model list, so they stay unreachable until a group lists them. Adding an
  id to a group's model list is the only step needed.
- To turn Devin into a peer provider for these ids instead of overflow, lower the account priority below
  the competing accounts (smaller number = more preferred). Do this deliberately: it changes how
  standard-name traffic is distributed between our OpenAI accounts and Devin's plan credits.
- The chat-completions stream path does not rewrite a mapped upstream model back to the requested id,
  which is why the sidecar aliases the standard ids directly and the account mapping stays identity
  based. If the gateway ever rewrites mapped models on that path, a distinct upstream id
  (`devin-gpt-5.5`) would restore the extra provenance in `upstream_model` without changing the
  client-visible name.

## 5. Codex CLI compatibility (2026-09-23)

- [x] 5.1 Verify Codex CLI (`0.155.1`) end-to-end through the gateway against a Devin-only route:
  a simple turn and a real tool-calling task (create a file and read it back) both succeed for `swe-2`
  and `gpt-5.5`.
- [x] 5.2 Capture Codex's real request body and identify its tool shape: tools are declared as
  `input[].type = "additional_tools"` carrying `type: "namespace"` groups (`functions`, `clock`,
  `collaboration`), alongside `client_metadata`, `reasoning.context` and `text.verbosity`. The gateway
  flattens the namespaces for external bridges (`flattenOpenAIResponsesNamespaces`) and restores the
  names on the way back, which is what makes tool calls work on the Devin route at all.
- [x] 5.3 Replay bisect at the sidecar: the raw namespaced body is rejected for every model (403
  `permission_denied`, "MCP configuration issue"); removing `additional_tools` makes the same body
  succeed; a single simple function/custom tool succeeds; the full 11-tool Codex bundle is rejected even
  for `gpt-5.5` (502 `invalid_argument` / 403), so the bundle itself is what Devin cannot digest.
- [x] 5.4 Model matrix through the gateway with real Codex CLI requests: `swe-2`, `swe-1-7`,
  `swe-1-6`, `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex` work; `gpt-6-astra`, `gpt-5.6-sol`,
  `gpt-5.6-luna`, `gpt-5.6-terra` are refused by Devin (403) when the Codex tool bundle is present,
  yet the same models answer reduced requests; `gpt-4.1` is blocked by our own pricing gate.

### Sidecar robustness fix (applied)

- The single-credential sidecar marked the credential unavailable after the first model error, so one
  bad request blacked out the whole Devin route until restart. `config.yaml` now sets
  `disable-cooling: true`, `request-retry: 1`, `max-retry-credentials: 1`, `max-retry-interval: 0`,
  `transient-error-cooldown-seconds: -1`; failure handling belongs to the gateway's account state and
  failover instead (backup: `config.yaml.bak-pre-cooling`).

### Operational note

- Restarting the sidecar while the gateway's scheduled diagnostics run makes the probe fail with `EOF`,
  which marks the account temporarily unschedulable (~10 minutes, reason
  "scheduled diagnostics failed"). A successful account test clears the state. Avoid sidecar restarts
  during diagnostic windows.

### Open questions

- Which element of Codex's 11-tool bundle Devin refuses is still unisolated (dots in names, the
  `custom`-typed `functions.exec`, or the `clock`/`collaboration` agent tools all survive individually
  in smaller combinations). A gateway-side or sidecar-side tool-slimming pass for Devin routes is the
  candidate workaround; it needs its own proposal.
- `gpt-4.1` remains mapped but unpriced; either add a price entry or drop it from the alias/mapping set.
- Whether to keep exposing `gpt-6-astra` / `gpt-5.6-*` on Devin at all, given Codex-shaped traffic is
  refused for them.

## 6. Converge the exposed set (2026-09-23)

- [x] 6.1 Drop the alises and mapping entries that Codex-shaped traffic cannot use: `gpt-6-astra`,
  `gpt-5.6-sol`, `gpt-5.6-luna`, `gpt-5.6-terra` (Devin refuses the Codex tool payload) and `gpt-4.1`
  (our own pricing gate). `swe-1-6-slow` was dropped from the sidecar catalog as well because Devin
  answers it with a 403.
- [x] 6.2 Resulting Devin surface: sidecar aliases and account mapping both list exactly
  `swe-2`, `swe-1-7`, `swe-1-6`, `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex`; account path
  probes through the gateway answer 200 for all of them.

## 7. Tool-slimming prototype (2026-09-23)

- [x] 7.1 Capture Codex 0.155.1's request shapes: for models it treats as standard (`gpt-5.5`) it sends a
  flat tool list (`exec_command`, `write_stdin`, `apply_patch` as `custom`, `view_image`, goal tools,
  `tool_search`, `web_search`); for `gpt-6-astra` it sends namespaced tools
  (`functions`/`clock`/`collaboration`), where `functions.exec` is declared `type: "custom"`.
- [x] 7.2 Replay the gateway-equivalent outbound payload (`additional_tools` → top-level `tools`, names
  flattened as `namespace__name`) against Devin: full set → 403; the same set without the `custom`
  `functions.exec` tool → **200**; the `custom` tool alone → 403; no tools → 200; our own single custom
  tool → 200. The trigger is Codex's freeform exec tool declaration, not the model id and not tools in
  general.
- [x] 7.3 Establish that the client side cannot be fixed by configuration: `experimental_use_unified_exec_tool=false`
  still produces the namespaced payload with the `custom` exec tool.
- [x] 7.4 Implement the fix in the gateway. The real trigger turned out to be **tool-description
  length**, not `custom` tools: with the faithful 59KB Codex body, Devin answers a single tool whose
  description is 4096 characters with 200 and one of 5120 characters with the misleading
  `permission_denied` "MCP configuration issue"; dropping or replacing the descriptions fixes it, while
  dropping the parameters does not. Codex declares its `functions.exec` description at 10,520
  characters. The gateway therefore truncates over-long tool descriptions for external bridges
  (`truncateExternalBridgeToolDescriptions`, cap 4000 runes, cut on a line boundary, marker appended)
  alongside the custom-tool downgrade and namespace flattening. Implemented in
  `apicompat.DowngradeResponsesCustomTools` + `service/openai_responses_namespace.go` with unit tests
  (including a round-trip that asserts `function_call` for a downgraded tool is restored as
  `custom_tool_call`).
- [x] 7.5 Re-expose the four models (account mapping to the namespaced upstream ids; no sidecar alias
  needed) and verify with real Codex CLI runs: `gpt-6-astra` answered a simple turn and completed a real
  two-step shell task (create + read back a file) through Devin, and `gpt-5.6-sol` / `-luna` / `-terra`
  each answered. All four also pass the gateway account probe on the deployed build.

### What the truncation drops (and its blast radius)

- Only **one** of the eleven Codex tools exceeds the cap: `functions__exec` (10,520 → 4,000 characters).
  Every other tool stays intact (next longest is `collaboration__spawn_agent` at 1,994).
- The dropped 6,520 characters are the TypeScript API appendix documenting the *nested* tools reachable
  from the exec script (`tools.exec_command`, goal helpers, …). The retained head keeps the operational
  contract: what exec is for, the V8/async execution model, the `tools` global and argument conventions,
  the `// @exec: {"yield_time_ms":…,"max_output_tokens":…}` pragma and its defaults.
- Scope is external bridges only (`account.IsExternalBridge()`); OAuth and API-key upstreams keep the
  full descriptions. After the change, Codex CLI completed a real shell task on `gpt-6-astra`, and the
  previously working models (`gpt-5.5`, `swe-2`, …) still answer.
- Residual risk: the model may occasionally mis-guess a nested tool's parameters; the line-boundary cut
  plus the explicit truncation marker keep it from acting on a half-written sentence. If it bites, the
  cap can be raised to the measured 4096 limit or a compact signature summary can be appended instead.

## 8. Devin support

- [x] 8.1 Draft the ticket with the failing trace IDs, the exact payload and the isolation matrix
  (`devin-support-ticket.md`) for the account owner to send through Devin's support channel.

## 9. Complete the Devin model map (2026-09-23)

- [x] 9.1 Enumerate the sidecar's 40-model catalog and translate each Devin id to our naming
  (numeric version segments use dots: `devin/gpt-5-5` → `gpt-5.5`, `devin/gemini-3-8-flash` →
  `gemini-3.8-flash`), then keep only the ids that our price table can bill.
- [x] 9.2 Add sidecar aliases so every mapped model is reachable under the client-facing id (23 bare
  ids in total) and set the account mapping to identity, which also removes the earlier
  `devin/<id>` upstream mapping for `gpt-6-astra` / `gpt-5.6-*` (that mapping leaked the namespaced id
  into streaming chat-completions responses).
- [x] 9.3 Map the platform's `swe-2` tiers: `swe-2-high` / `swe-2-medium` / `swe-2-max` → `swe-2`.
  The sidecar catalog only carries `swe-2`; the reasoning tier is taken from the request payload, so the
  tiered client names stay usable and are still recorded as the requested model.
- [x] 9.4 Verify through the deployed gateway: account probes for `claude-opus-4.8`, `gemini-3.8-flash`,
  `grok-4.5`, `kimi-k3`, `deepseek-v4-pro`, `swe-2-max` all complete, and public group-4 requests for
  `claude-opus-4.8` / `gemini-3.8-flash` answer 200 with the client-facing model while billing lands on
  account 126 with a non-zero cost.

### Completed map (26 entries on account 126)

| group | models |
| --- | --- |
| Codex/Devin family | `swe-2`, `swe-1-6`, `swe-1-7`, `swe-2-high`, `swe-2-medium`, `swe-2-max` |
| GPT family | `gpt-5.3-codex`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.5`, `gpt-5.6-sol`, `gpt-5.6-luna`, `gpt-5.6-terra`, `gpt-6-astra` |
| Claude family | `claude-opus-4.6`, `claude-opus-4.7`, `claude-opus-4.8`, `claude-opus-5` |
| Gemini family | `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash` |
| Others | `deepseek-v4-flash`, `deepseek-v4-pro`, `grok-4.5`, `grok-4.6`, `kimi-k3` |

### Pricing verification (2026-09-23) — the earlier exclusion list was wrong

The first pass compared Devin ids against the catalog's `fallback_pricing` table only and concluded that
19 ids had no price. That table is not the whole runtime price set: a live sweep of real requests showed
that **12 of them bill a non-zero cost**, so they were mapped. The corrected picture:

| outcome | models |
| --- | --- |
| billable, now mapped | `claude-sonnet-5`, `claude-sonnet-4.5`, `claude-sonnet-4.6`, `claude-haiku-4.5`, `claude-5-fable`, `claude-fable-5.1`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash`, `kimi-k2.7`, `gemini-3-flash`, `gemini-3.5-flash` (plus `kimi-k2.6` and `deepseek-v4.1-flash`, which the first pass mis-translated) |
| no price at runtime | `gpt-4.1`, `nemotron-3-ultra` — both record cost 0, so they stay unmapped until a price entry exists (the repo's reference file has official `gpt-4.1` rates; `nemotron-3-ultra` has none) |
| refused upstream | `swe-1.6-slow` (Devin answers 403) |
| duplicate spelling | `swe-1.6` / `swe-1.7` are the dot spelling of the already-mapped `swe-1-6` / `swe-1-7` |

The map now holds **40 entries**, and the sidecar publishes 37 bare ids (the extra 12 aliases were added
alongside the earlier ones).

### Operational findings from the sweep

- **A UI save rewrote the model map.** A `PUT /api/v1/admin/accounts/126` from the operator's browser
  reduced a 26-entry map to 16 entries (every `claude-*`, `gemini-*`, `deepseek-*`, `kimi-k3` mapping was
  dropped) while the API itself stores maps verbatim (verified: a 21-entry map round-tripped unchanged).
  The account form's model-mapping editor therefore prunes entries it does not know about — worth fixing
  before relying on the UI for this account.
- **An upstream 403 puts the account in a ~10 minute cooldown.** Probing `swe-1.6-slow` (Devin 403) set
  `temp_unschedulable_until` with reason "OpenAI 403 temporary cooldown (1/3)", which made *every* model
  on that account answer 404 until a successful account probe cleared it. Do not probe upstream-refused
  models on a production account; and expect the same effect from any transient Devin 403.

## 10. Pricing data on the host and dropping unpriced mappings (2026-09-23)

- [x] 10.1 Publish our own price table as a data file: `backend/resources/model-catalog/catalog.json`'s
  `fallback_pricing` (85 entries, all USD token rates) was exported in the LiteLLM layout and installed at
  `<resources mount>/model-pricing/model_prices_and_context_window.json`, which is exactly
  `pricing.fallback_file`. Startup now logs `[Pricing] Using fallback file: …` and
  `[Pricing] Loaded 63 models from …` instead of relying only on the values compiled into the binary.
- [x] 10.2 Give the service a writable data directory. The container was recreated with
  `PRICING_DATA_DIR=/app/data/pricing` (everything else identical: image, entrypoint, cmd, restart policy,
  ports, mounts, network, and the other 114 env vars — verified by comparing `docker inspect`). The
  fallback file is now copied there and subsequent starts load
  `[Pricing] Loaded 63 models from /app/data/pricing/model_pricing.json` with no error, where previously
  every start logged `Failed to copy fallback: read-only file system` and
  `Initial load failed, using fallback: invalid pricing url: url is required`.
- [x] 10.3 Rollback reference captured at
  `/opt/sub2api/backups/inspect-sub2api-api-candidate-0.1.391-acct-0383eff2f-<ts>.json`, and the sidecar
  config backups remain (`config.yaml.bak-pre-*`).
- [x] 10.4 Sweep every mapped model with a real request and drop the ones that record zero cost. Result:
  38 of the 40 bill a non-zero amount; only `swe-1-6` and `swe-1-7` came back as `0.0000000000`, so both
  were removed from `model_mapping` and their sidecar aliases were deleted (they now answer 404 and the
  catalog lists them as `devin/swe-1-6` / `devin/swe-1-7` again). `gpt-4.1` and `nemotron-3-ultra` were
  never mapped, so nothing else needed removal.
- [x] 10.5 Regression after the change: `swe-2`, `gpt-5.5`, `claude-sonnet-5` and `gemini-3.8-flash` all
  answer 200 and bill non-zero (`gpt-5.5` on the OAuth account, the Devin models on account 126).

### Final state

| item | value |
| --- | --- |
| account 126 `model_mapping` | 38 entries (Devin catalog minus unpriced/refused ids) |
| sidecar published bare ids | 35 aliases |
| pricing file | `<resources>/model-pricing/model_prices_and_context_window.json` (85 entries, ours) |
| pricing data dir | `/app/data/pricing` (writable, holds the loaded copy) |
| remote pricing refresh | still disabled by choice; enabling it would let the upstream mirror override our curated rates, so it needs a deliberate decision |

