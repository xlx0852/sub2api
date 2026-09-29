# Change: Add OAuth upstream model sync for OpenAI and Grok

## Why

The admin "同步上游模型" button (`POST /api/v1/admin/accounts/:id/models/sync-upstream`) is shown for
every Anthropic / OpenAI / Gemini / Antigravity / Grok account whose platform is in the frontend's
`upstreamSyncPlatforms` set, but the backend only implements the **api-key** variants for OpenAI and
Grok:

- `buildOpenAIUpstreamModelsRequest` rejects anything that is not `AccountTypeAPIKey`
  (`Unsupported OpenAI account type for upstream model sync: oauth`).
- `buildGrokUpstreamModelsRequest` rejects anything that is not `AccountTypeAPIKey`
  (`Unsupported Grok account type for upstream model sync: oauth`).

Both rejections are `400` responses with an `Unsupported ...` message. Live production accounts
confirm this is not a corner case: the running deployment has `openai|oauth|5` accounts and
`grok|oauth|9` accounts (anthropic has 3 api-key, 0 oauth; there is no gemini oauth and no kimi
account), so **14 of the 22 live accounts are OAuth accounts for which the button is guaranteed to
fail**. The models those accounts can actually serve therefore have to be typed in by hand.

The reason the api-key code path cannot be reused is authentication, not policy: ChatGPT OAuth
tokens are not accepted by `https://api.openai.com/v1/models`, and the api-key path sends the wrong
credential for Grok OAuth. The upstreams that *do* accept OAuth tokens were verified live on
2026-09-30 (see `design.md` for the full method):

- `GET https://chatgpt.com/backend-api/codex/models?client_version=<v>` with the OAuth token returns
  `200`; `GET https://api.x.ai/v1/models` with a Grok OAuth token returns `200`.
- The manifest is **plan- and client-version-scoped**. Holding the request constant and varying only
  `client_version` on one live `pro` account: `0.140.0` -> 1 visible model, `0.150.0` -> 4,
  `0.153.3`/`0.153.4` -> 5, `0.156.0`/`0.157.0` -> 7, `0.159.0` -> 8. Omitting the parameter returns
  `400`. Two entries (`gpt-reserve`, `codex-auto-review`) always carry `visibility: hide`.
  The implementation must therefore send the deployment's effective Codex client version instead of
  a constant, and must filter hidden entries.

The remaining OAuth surfaces are retired by upstream decisions and cannot be rehabilitated here.

## What Changes

- **OpenAI OAuth (incl. agent-identity)**: an OpenAI account with a non-api-key type syncs through
  the Codex models manifest (`GET /backend-api/codex/models`) using the token provider's
  refresh-aware access token, and returns the manifest's model IDs.
- **Grok OAuth**: a Grok OAuth account syncs through `GET {base_url}/v1/models` with a
  provider-refreshed Bearer token; the api-key path keeps using the stored `api_key`.
- **Unchanged rejections**: Anthropic OAuth/setup-token (login retired), Gemini Code Assist OAuth
  with a `project_id` (retired), Gemini service-account (retired Vertex), Kimi OAuth (no verified
  listing endpoint), and all remaining platform/type combinations keep returning the existing
  `configuration` / `unsupported` messages instead of attempting a request.
- **No frontend change**: the selector already merges the sync result into the whitelist (it never
  replaces it), so a successful OAuth sync can only add models; the still-unsupported combinations
  keep showing the existing actionable message.

## Impact

- Affected specs: `upstream-model-sync` (new capability; no existing spec covers this surface).
- Affected code: `backend/internal/service/upstream_models.go`,
  `backend/internal/service/account_test_service.go` (dependency field),
  `backend/internal/service/wire.go` (inject the existing OpenAI gateway service),
  plus service tests.
- No database migration. No API shape change: the response is still `{"models": [...]}` and the
  error contract (`configuration` / `unsupported` → 400, `upstream` → 502) is unchanged.
- No pricing, catalog, or whitelist-semantics change.

Expected operator-visible result: with the production deployment's current effective client version
(`0.153.4`) a successful sync returns 5 models; `gpt-6-sol` / `gpt-6-luna` and `gpt-6.1-sol` require
a newer advertised client version (`>=0.156.0` and `>=0.159.0` respectively), which is an operator
setting (`openai_codex_client_version`, or the automatic version sync), not part of this change.
