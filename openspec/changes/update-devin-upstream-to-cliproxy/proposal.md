# Change: Move the Devin upstream to the CLIProxyAPI sidecar

## Why

Our Devin traffic runs through the `leokun123/devin-2api` sidecar
(`/opt/sub2api/bridges/devin/releases/…`, container `sub2api-devin-bridge`, reached by account 126 at
`http://sub2api-devin-bridge:8080`). Operationally it is the fragile leg of the pool:

- its configuration pins a single Devin session token and a single model (`devin.model: glm-5-2`,
  `force_http1: true`), so a token rotation is a manual host edit and a second Devin account cannot be
  added at all;
- it exposes whatever model list its own build happened to embed — no registry, no refresh;
- it has no credential record: nothing tracks plan, quota signals, or session state, and a rejected
  token surfaces as a plain upstream error.

The CLIProxyAPI implementation operators compared against handles the same Devin Connect transport in a
native Go executor (`internal/runtime/executor/devin_executor.go`, ~2.5k lines, with its own wire
helpers, retry/`Retry-After` handling, tool translation and unit tests), keeps credentials as
`auths/devin-*.json` records that carry profile, plan and quota signals, ships a 40-model Devin catalog
with a refresher, and supports several Devin accounts behind one endpoint. It is the same idea as our
sidecar, with the credential and retry lifecycle we are missing.

The switch is a sidecar swap, not a gateway rewrite: our external-bridge path only requires an
OpenAI-compatible endpoint, and CLIProxyAPI serves both `/v1/chat/completions` and `/v1/responses`.

## What Changes

- Build the Devin sidecar from the pinned CLIProxyAPI release (`v7.3.13`, official linux/amd64 artifact
  verified against its `checksums.txt`) instead of the `devin-2api` image, and run it as
  `sub2api-devin-cliproxy` on the existing private bridge network.
- Import the existing Devin session token into a CLIProxyAPI auth file with the headless flow
  (`--devin-login --no-browser`, token piped in, never printed), so the credential becomes a managed
  record with profile/plan/quota enrichment rather than a value baked into a config file.
- Keep a client API key on the sidecar, exposed to Sub2API, and keep the sidecar privately reachable
  (no public listener; the gateway reaches it by container name).
- Repoint Devin account 126 (`platform=openai`, `external_bridge_kind=devin`) at the new sidecar:
  `credentials.base_url` and `credentials.api_key` only — the model id (`swe-2-high`), model mapping
  (`swe-2 → swe-2`) and the client-facing gateway endpoint stay unchanged.
- Keep the old `sub2api-devin-bridge` container and its release directory untouched as the rollback
  path, and record the exact previous credential values in the deploy record.
- Verify parity before cutover: model list, non-streaming and streaming chat completions, a
  `/v1/responses` call, tool calling, and usage/billing rows through the public gateway afterwards.

## Impact

- Affected specs: `external-provider-bridges` (modified: the Devin sidecar implementation and its
  credential lifecycle).
- Affected infrastructure: a new sidecar release directory and container,
  `/opt/sub2api/bridges/devin-cliproxy/`, plus one account-credential update for account 126.
- No gateway code change: the external-bridge forwarding, usage projection and billing paths are
  unchanged, and `swe-` model handling already special-cases Devin.
- Rollback is a single credential update back to `http://sub2api-devin-bridge:8080` and its existing
  API key, with the old container left running.
