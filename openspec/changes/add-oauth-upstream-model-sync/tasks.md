# Tasks: Add OAuth upstream model sync for OpenAI and Grok

## 1. OpenAI OAuth model listing

- [x] 1.1 Extend the manifest entry type used while parsing Codex manifests with the `visibility`
      field (keep the existing scoring behavior unchanged).
- [x] 1.2 Add a helper that turns a Codex manifest body into a sorted, deduplicated model ID list:
      prefer `slug`, fall back to `id`, drop entries whose `visibility` is `hide` (case-insensitive),
      keep entries with a missing/unknown `visibility`.
- [x] 1.3 In `upstream_models.go`, route non-api-key OpenAI accounts to
      `OpenAIGatewayService.FetchCodexModelsManifest` instead of the current rejection, passing the
      effective Codex client version (empty string falls back to the compiled constant).
- [x] 1.4 Map failures to the existing error kinds: unavailable gateway service or unreadable token
      → `configuration`; upstream HTTP error / invalid JSON → `upstream`; empty result → the
      existing empty-list message.

## 2. Wiring

- [x] 2.1 Add an `openAIGatewayService *OpenAIGatewayService` field to `AccountTestService`.
- [x] 2.2 Assign it in `ProvideAccountTestService` (the OpenAIGatewayService value is already a
      parameter there) and set the field on the service instance.
- [x] 2.3 Confirm no other constructor path leaves the field nil without a guard (all new call
      sites must nil-check and return an `unsupported`/`configuration` error instead of panicking).

## 3. Grok OAuth model listing

- [x] 3.1 In `buildGrokUpstreamModelsRequest`, accept Grok OAuth accounts: obtain the token from
      `grokTokenProvider` (refresh-aware) and use `account.GetGrokOfficialBaseURL()`; keep the
      existing api-key branch byte-for-byte.
- [x] 3.2 Guard the missing-provider and missing/expired-token cases with `configuration` errors.
- [x] 3.3 Keep the existing api-key and unsupported-type messages unchanged.

## 4. Tests

- [x] 4.1 OpenAI OAuth: manifest with `list` + `hide` entries → only visible `slug`s, sorted and
      deduplicated; `id` fallback covered.
- [x] 4.2 OpenAI OAuth: token retrieval failure → `configuration`; non-2xx and invalid JSON →
      `upstream`; empty list → empty-list message.
- [x] 4.3 OpenAI api-key: payload and behavior unchanged (regression test).
- [x] 4.4 Grok OAuth: request carries `Bearer` token and the account base URL; Grok api-key:
      unchanged; expired token without refresh → `configuration`.
- [x] 4.5 Anthropic OAuth/setup-token, Gemini OAuth with `project_id`, Gemini service-account and
      Kimi OAuth still return their existing unsupported/configuration messages and perform no
      network call.
- [x] 4.6 Run `go build ./...` and the touched service packages' tests.

## 5. Documentation

- [x] 5.1 `openspec validate add-oauth-upstream-model-sync --strict` passes.
- [x] 5.2 Note the client-version gating and the "results are plan- and version-scoped" behavior
      for operators. There is no operator-facing document for the sync button (the button's
      behavior is only described in code), so the note lives in `proposal.md` (Impact).
