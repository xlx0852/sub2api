# Change: Serve Devin's GPT family under our standard model ids

## Why

The Devin upstream we just moved to exposes a 40-model catalog, and nine of those entries are GPT-family
models that the Devin plan on the current credential can actually serve (verified by calling each one:
`gpt-5-4`, `gpt-5-4-mini`, `gpt-5-3-codex`, `gpt-5-5`, `gpt-6-astra`, `gpt-5-6-sol`, `gpt-5-6-luna`,
`gpt-5-6-terra`, `gpt-4-1`). The sidecar names them `devin/gpt-5-5` and friends, which no client asks
for, so that capacity is unreachable through the gateway.

Operators want the standard OpenAI ids to be the ones that work: a client asking for `gpt-5.5` should be
able to be served by Devin's route when the OpenAI accounts for that model cannot take the request.

## What Changes

- Alias the nine Devin GPT entries to our standard ids on the sidecar
  (`devin/gpt-5-5` → `gpt-5.5`, …) with response-model rewriting, so the sidecar accepts exactly the ids
  our clients use.
- Register those ids in the Devin account's `model_mapping` as identity entries, which is what makes the
  account a candidate for them.
- Place the Devin account in the **lowest scheduling priority tier** so the existing accounts keep
  serving these ids first and Devin is only reached when they are unavailable, rate-limited, or at
  capacity. Flipping the account back to a competitive priority later turns it into a peer provider
  without touching anything else.
- Keep the account's existing `swe` mapping, model, group membership, and status unchanged.

## Impact

- Affected specs: `external-provider-bridges` (the Devin sidecar now publishes standard ids) and
  `account-usage-presentation` (unchanged behaviour, but the standard ids inherit their existing price
  entries and audit columns).
- Affected configuration: sidecar `oauth-model-alias` entries plus account 126 `model_mapping` and
  `priority`. No gateway code change.
- Pricing: because the client-facing id stays the standard one, the existing price entries apply and the
  call is billed normally — unlike mapping onto a namespaced id, which has no price and records cost 0.
- Capacity that only exists in groups whose model list already includes the id becomes reachable
  immediately (`gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini` today); the remaining ids are wired but stay
  unreachable until a group lists them.
- Devin calls consume Devin plan credits, not our upstream quotas, and Devin's available model set varies
  by plan (`swe-1-6-slow` answers 403 `permission_denied` on this credential).
