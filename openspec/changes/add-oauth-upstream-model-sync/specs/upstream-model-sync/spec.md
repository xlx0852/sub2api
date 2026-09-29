# upstream-model-sync Specification

## ADDED Requirements

### Requirement: OpenAI OAuth Model Discovery
The system SHALL list models for an OpenAI OAuth account from the ChatGPT Codex models manifest
(`GET https://chatgpt.com/backend-api/codex/models`) using the account's refresh-aware access token,
instead of rejecting the account type, because the OAuth token is not accepted by the platform
model-list API.

#### Scenario: Synchronizing an OpenAI OAuth account
- **GIVEN** an OpenAI account with account type `oauth` and a valid or refreshable credential
- **WHEN** an administrator requests upstream model synchronization for that account
- **THEN** the system requests the Codex models manifest with the account's access token and its
  `chatgpt-account-id`
- **AND** returns the manifest model IDs as a sorted, deduplicated list

#### Scenario: Excluding internal models
- **GIVEN** a manifest entry whose `visibility` is `hide`
- **WHEN** the model list is built
- **THEN** that entry is excluded from the result
- **AND** an entry without a `visibility` field is treated as visible

#### Scenario: Missing or unrefreshable credential
- **GIVEN** an OpenAI OAuth account whose access token cannot be obtained or refreshed
- **WHEN** model synchronization is requested
- **THEN** the system returns a configuration error without contacting upstream
- **AND** reports the account type in the message without any credential material

#### Scenario: API-key accounts are unaffected
- **GIVEN** an OpenAI account with account type `api_key`
- **WHEN** model synchronization is requested
- **THEN** the system keeps listing models from the platform model-list API with the stored API key

### Requirement: Codex Manifest Client Version Scoping
The system SHALL send the deployment's effective Codex client version when requesting the Codex
models manifest, because the manifest is scoped by client version and the returned entitlement set
must match the identity the gateway advertises when serving those models.

#### Scenario: Effective client version is used
- **GIVEN** an effective Codex client version configured by an administrator override or by the
  automatic version sync
- **WHEN** upstream model synchronization requests the manifest
- **THEN** the request carries that version as the `client_version` query parameter

#### Scenario: No configured version
- **GIVEN** no administrator override and no synchronized version
- **WHEN** upstream model synchronization requests the manifest
- **THEN** the compiled default Codex client version is used

### Requirement: Grok OAuth Model Discovery
The system SHALL list models for a Grok OAuth account from its configured base URL's `/v1/models`
endpoint using a refresh-aware Bearer token, while keeping the existing API-key behavior unchanged.

#### Scenario: Synchronizing a Grok OAuth account
- **GIVEN** a Grok account with account type `oauth` and a valid or refreshable credential
- **WHEN** an administrator requests upstream model synchronization for that account
- **THEN** the system requests `/v1/models` on the account's official base URL with a Bearer token
  obtained from the Grok token provider
- **AND** returns the deduplicated upstream model IDs

#### Scenario: Grok API-key accounts are unaffected
- **GIVEN** a Grok account with account type `api_key`
- **WHEN** model synchronization is requested
- **THEN** the system requests `/v1/models` with the stored API key as before

#### Scenario: Expired token without refresh material
- **GIVEN** a Grok OAuth account whose access token is expired and has no refresh token
- **WHEN** model synchronization is requested
- **THEN** the system returns a configuration error without contacting upstream

### Requirement: Retired OAuth Paths Stay Explicitly Rejected
The system SHALL NOT attempt network requests for OAuth or account-type combinations whose listing
capability is retired or unverified upstream, and SHALL keep returning the existing
configuration/unsupported errors for them.

#### Scenario: Retired and unverified combinations
- **GIVEN** an Anthropic `oauth`/`setup_token` account, a Gemini OAuth account carrying a
  `project_id`, a Gemini service-account, or a Kimi OAuth account
- **WHEN** model synchronization is requested
- **THEN** the system returns the existing configuration or unsupported error for that combination
- **AND** performs no upstream request

### Requirement: Sync Result and Credential Hygiene
The system SHALL return only model identifiers from the upstream listing, SHALL keep the existing
error contract (configuration/unsupported errors as client errors, upstream failures as gateway
errors), and SHALL NOT include credentials, tokens or raw upstream bodies in responses or logs.

#### Scenario: Non-success upstream response
- **GIVEN** an upstream model listing that fails or returns unparseable content
- **WHEN** model synchronization is requested
- **THEN** the system returns the existing gateway error with a category-based message and the
  numeric upstream status code only

#### Scenario: Repeated synchronization is stable
- **GIVEN** an unchanged upstream listing
- **WHEN** model synchronization is requested twice
- **THEN** both responses contain the same model identifiers in the same order
