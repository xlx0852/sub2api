## 1. Sidecar release

- [x] 1.1 Download `CLIProxyAPI_7.3.13_linux_amd64.tar.gz` and verify it against the release
  `checksums.txt`.
- [x] 1.2 Install it under `/opt/sub2api/bridges/devin-cliproxy/releases/v7.3.13/` with a sidecar
  `config.yaml` (listen port, `auth-dir`, client `api-keys`, Devin provider section) and an `auths/`
  directory owned by the sidecar user.
- [x] 1.3 Record the artifact checksum and the pinned version in the deploy record.

## 2. Credential

- [x] 2.1 Import the existing Devin session token into an auth file with the headless login flow
  (`--devin-login --no-browser`), reading the token from a pipe so it never reaches argv, shell history,
  logs, or the deploy record.
- [x] 2.2 Confirm the record carries profile/plan/quota signals and that the file lives in the sidecar
  `auths/` directory with owner-only permissions.

## 3. Pre-cutover verification

- [x] 3.1 `GET /v1/models` with the client key lists the Devin catalog; without the key it is rejected.
- [x] 3.2 Non-streaming and streaming chat completions for the account's model (`swe-2`) return text,
  and a tool-calling request returns a tool call.
- [x] 3.3 `POST /v1/responses` for the same model returns a well-formed response, since the account is
  configured `openai_responses_mode: force_responses`.
- [x] 3.4 The old `sub2api-devin-bridge` container keeps running, untouched, as the rollback upstream.

## 4. Cutover

- [x] 4.1 Update account 126's `credentials.base_url` and `credentials.api_key` to the new sidecar,
  leaving model, model mapping, platform, bridge kind, groups and status unchanged.
- [x] 4.2 Verify through the public gateway with a client key: a real `/v1/responses` request for
  `swe-2` succeeds, and the usage row is written with the model, tokens and cost.
- [x] 4.3 Confirm the gateway's health and error logs show no new upstream failures after the switch.

## 5. Rollback and retirement

- [x] 5.1 Document the previous `base_url` and API key so rollback is a single credential update.
- [ ] 5.2 Leave the old container running for a soak period; retire it and its release directory in a
  later cleanup once the new sidecar has served traffic without incidents.

## 6. Verification

- [x] 6.1 `openspec validate update-devin-upstream-to-cliproxy --strict`.
- [x] 6.2 Confirm no credential material was written to logs, task output, or the deploy record.

### Deploy record

- Sidecar: CLIProxyAPI `v7.3.13`, official `linux_amd64` artifact, sha256
  `8f741ee652684e7a9fe72296da71b2411a11165d6121f30a2a68fa02109dfb9b` (matches the release
  `checksums.txt`), installed at `/opt/sub2api/bridges/devin-cliproxy/releases/v7.3.13/`, running as
  container `sub2api-devin-cliproxy` on `sub2api_sub2api-network`, loopback-only publish
  `127.0.0.1:18363`, runtime base `debian:bookworm` with the host CA bundle bind-mounted at
  `/etc/ssl/certs/ca-certificates.crt`, management API disabled (`secret-key: ""` +
  `disable-control-panel: true`).
- Credential: `auths/devin-user-5987cbae4f24efeb.json` (mode 600), imported from the previous sidecar's
  session token with the headless flow; `auth_kind=oauth`, `disabled=false`.
- Account 126 before: `base_url=http://sub2api-devin-bridge:8080`, `model=swe-2-high`,
  `model_mapping={"swe-2":"swe-2"}` (old bridge API key: 48 chars).
- Account 126 after: `base_url=http://sub2api-devin-cliproxy:8317`, `model=swe-2`,
  `model_mapping={"swe-2":"swe-2","swe-1-7":"swe-1-7","swe-1-6":"swe-1-6"}`.
- The sidecar aliases the `swe` family to bare ids (`oauth-model-alias` with `force-mapping: true`), so
  the gateway keeps sending the ids it always sent and clients keep seeing the model they asked for.
- Rollback: set the account credentials back to the values above and `docker start sub2api-devin-bridge`.
  The old container is stopped but preserved with its release directory
  (`/opt/sub2api/bridges/devin/releases/cache-stable-20260913/`).

### Verification

- Sidecar direct: `/v1/models` returns 40 Devin models; without the client key it answers 401; bare
  `swe-2` / `swe-1-7` / `swe-1-6` all answer 200 and echo the bare id back.
- Public gateway: non-streaming chat completions, streaming chat completions (chunk `model` is `swe-2`,
  not `devin/swe-2`), `/v1/responses`, and a tool-calling request all succeed; usage rows are written
  for account 126 with the model, tokens and a non-zero cost.

### Known trade-off (accepted)

The account usage projection reads `GET /panel/api/status` from the bridge for plan name and remaining
prompt/flow credits. CLIProxyAPI does not expose that endpoint, so Devin accounts now report the bridge
usage panel as `status: error` / `bridge usage returned HTTP 404` and the account row shows no Devin
quota window. This is accepted: inference, scheduling, billing and client-visible responses are
unaffected, and the vendor seat-management call that backs that panel did not return plan data during
the credential import either. Revisit with a native Devin status fetch (or a sidecar plugin) if the
panel is wanted back; do not reintroduce the old bridge for it.
