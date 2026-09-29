# Design: OAuth upstream model sync

## Context

`FetchUpstreamSupportedModels` (`backend/internal/service/upstream_models.go`) dispatches on platform
and then rejects every non-api-key account for OpenAI and Grok. The frontend shows the button for
`anthropic | openai | gemini | antigravity | grok` regardless of account type
(`frontend/src/components/account/ModelWhitelistSelector.vue`), so OAuth accounts get a button that
always answers `400 Unsupported ... account type for upstream model sync`.

Production account inventory (read-only, `deleted_at is null`) shows this is the main population:

| platform / type | count | upstream sync today |
| --- | --- | --- |
| anthropic / apikey | 3 | works (`/v1/models`) |
| openai / apikey | 2 | works (`/v1/models`) |
| grok / apikey | 1 | works (`/v1/models`) |
| **grok / oauth** | **9** | **rejected** |
| **openai / oauth** | **5** | **rejected** |
| antigravity / oauth | 2 | works (Cloud Code `FetchAvailableModels`) |

### Live verification of the OAuth endpoints

Both endpoints below were verified against production accounts with read-only `GET` probes on
2026-09-30. No token material was logged; only status codes and model identifiers were read.

**OpenAI (ChatGPT/Codex OAuth)** — `GET https://chatgpt.com/backend-api/codex/models?client_version=<v>`
with `Authorization: Bearer <oauth access_token>` + `chatgpt-account-id`, returns `200` and
`{"models":[{ ... , "slug": ..., "visibility": "list"|"hide", "supported_in_api": true, ...}]}`.
There is no usable alternative: the ChatGPT OAuth token is not accepted by the platform API
(`api.openai.com/v1/models`), which is why the api-key code path cannot be reused.

The response is **plan- and client-version-scoped**. Probing one `pro` account while varying only
`client_version` (the rest of the request held constant):

| `client_version` | visible `slug`s after excluding `visibility: hide` |
| --- | --- |
| `0.140.0` | `gpt-5.5` |
| `0.150.0` | + `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna` (4) |
| `0.153.3`, `0.153.4` | + `gpt-6-astra` (5) |
| `0.156.0`, `0.157.0` | + `gpt-6-sol`, `gpt-6-luna` (7) |
| `0.159.0`, `999.0.0` | + `gpt-6.1-sol` (8) |

`gpt-reserve` and `codex-auto-review` are present in every response with `visibility: hide`. Omitting
`client_version` returns `400`; `0.20.0` returns `200` with an empty list.

Current effective outbound version on the production host is `0.153.4`
(`settings.openai_codex_client_version_synced`; the panel override is empty), while the newest
upstream stable Codex release is `0.159.0`.

**Grok OAuth** — `GET {base_url}/v1/models` with `Authorization: Bearer <oauth access_token>`
returns `200` with the standard OpenAI-style `{"data":[{"id": ...}]}` list. Every live Grok OAuth
account stores `base_url = https://api.x.ai/v1`, i.e. the gateway already sends OAuth tokens to that
host for inference, so the token is accepted there.

**Retired OAuth surfaces (not implementable here):** Anthropic OAuth/setup-token sign-in was removed
(`anthropic oauth/setup-token sign-in has been retired; use anthropic api-key instead`); Gemini Code
Assist OAuth listing is unsupported and the Code Assist client package was deleted
(`LoadCodeAssist`/`OnboardUser` return `gemini code assist retired`); Gemini service-account is
retired Vertex; Kimi OAuth has no verified listing endpoint. These keep their current explicit
rejections.

## Decisions

**D1 — Reuse `OpenAIGatewayService.FetchCodexModelsManifest` instead of writing a new HTTP call.**
It already implements everything the sync needs: `resolveCredentialAccount` for shadow accounts,
`GetAccessToken` (cache + refresh) rather than a stale DB snapshot, agent-identity assertion
headers, `chatgpt-account-id` headers, proxy resolution via `resolveCodexManifestProxyURL`, the
3.5s upstream budget, and the `Originator`/`User-Agent` identity. A bespoke client in
`upstream_models.go` would duplicate auth-header construction, silently skip token refresh, and
drift from the gateway.

**D2 — Ask with the deployment's effective Codex client version.**
`client_version` selects entitlements upstream, and the gateway advertises
`SettingService.GetOpenAICodexClientVersion(ctx)` (panel override → auto-synced release → compiled
constant) when it actually serves those models. The sync passes that same value so the whitelist
reflects what this deployment can really serve. Passing a fabricated high version would list models
the deployment cannot serve under its declared identity.

**D3 — Exclude `visibility: hide`, treat a missing `visibility` as visible.**
Hidden entries (`gpt-reserve`, `codex-auto-review`) are internal Codex-side models, not models an
admin should put in a user-facing whitelist. The `hide` value is the only exclusion signal observed;
unknown or absent values stay included so a schema change cannot silently empty the result.

**D4 — Extract IDs from `slug`, fall back to `id`, then dedupe and sort.**
`codexManifestModelEntry`/`codexManifestEnvelope` already exist in
`openai_codex_models_service.go` for scoring manifests; the envelope type gains a `Visibility` field
and is reused, so there is one definition of the manifest shape. Sorting keeps the admin UI stable
between syncs.

**D5 — Grok branches on account type, not on a new provider.**
`AccountTestService` already holds `grokTokenProvider`; OAuth accounts obtain the token from
`GetAccessToken` (refresh-aware) and read the list from `account.GetGrokOfficialBaseURL()`. The
api-key branch keeps using the stored `api_key`, so both types end up at the same URL and parser.

**D6 — No frontend change.**
The selector already unions the sync result into the whitelist instead of replacing it, and the
remaining unsupported combinations already surface an actionable message (`... has been retired;
use ... api-key instead`). Gating the button by account type would be a larger change across every
call site for no functional gain, and is deliberately left out.

**D7 — Error contract unchanged.**
Missing/refresh-failed tokens are `configuration` errors (400), a `hide`-only or empty list is
`unsupported`/empty-list behavior as today, and upstream HTTP failures stay `upstream` (502). Error
messages carry only status codes and categories, never tokens.

## Rejected alternatives

- **Hide the button for OAuth accounts.** Fixes the confusion but leaves the model list manual for
  14 live accounts; the user asked for real capability.
- **Call `api.openai.com/v1/models` with the OAuth token.** Not accepted by the platform API.
- **Implement Anthropic/Gemini OAuth listing.** Impossible: retired upstream, not a client gap.
- **Reuse the client-facing `FetchPreferredCodexModelsManifest`.** It iterates/ranks many accounts
  and reads a group-scoped cache; the admin sync is about one specific account and must not report
  another account's entitlements.

## Risks / trade-offs

- The manifest is plan-scoped, so a `free`/limited account legitimately syncs fewer models than a
  `pro` account. Results are not comparable across accounts by design.
- Version gating (see table) means the sync stays conservative until the deployment's effective
  client version moves forward. With `0.153.4` in production, `gpt-6-sol`/`gpt-6.1-sol` will not
  appear even though the catalog knows them; raising `openai_codex_client_version` is an operator
  action, not part of this change.
- One extra upstream request per click (3.5s budget); the endpoint already serves the client-facing
  picker, so the load is unchanged in kind.
- The response body can be large (observed ~106 KB for 2 entries, ~361 KB for 7). The parser reads
  only the fields it needs.

## Open questions

1. Should the sync surface a hint when the manifest returns models the deployment's client version
   does not cover, or when the effective client version is older than the newest upstream release?
2. Should `gpt-reserve` / `codex-auto-review` ever be addressable through the whitelist (currently
   excluded by D3)?
