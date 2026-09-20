# antigravity Specification

## Purpose
限定 Antigravity 的 Gemini 模型范围、配额查询、传输隔离与思考签名回放合同。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: Antigravity model scope
The Antigravity gateway SHALL accept text Gemini model requests only. It SHALL preserve Gemini text generation, function calling, thinking configuration, OAuth authentication, project onboarding, quotas, retries, and streaming responses. Requests targeting Claude or image models SHALL be rejected with an explicit unsupported-model error.

#### Scenario: Gemini text request
- **WHEN** a request targets a supported `gemini-*` text model
- **THEN** the gateway forwards it through the Antigravity v1internal Gemini path and returns the requested JSON or SSE response

#### Scenario: Unsupported Claude or image request
- **WHEN** a request targets a Claude or image model
- **THEN** the gateway rejects it before upstream execution with an explicit unsupported-model error

### Requirement: CPA-compatible quota summary
The system SHALL query Antigravity quota windows through `retrieveUserQuotaSummary`, preserve group and bucket metadata, expose `remainingFraction` and reset times, and keep subscription tier and AI Credits from `loadCodeAssist`.

#### Scenario: Text Gemini quota refresh
- **WHEN** an administrator refreshes an eligible Antigravity account quota
- **THEN** the system returns text Gemini quota groups and buckets with remaining percentages and reset times while omitting Claude and image groups

### Requirement: Wire fingerprint and transport isolation
The system SHALL communicate with Antigravity upstream endpoints exclusively using HTTP/1.1 without advertising ALPN HTTP/2 (`h2`). Each Antigravity account SHALL maintain its own isolated connection pool so that different credentials never share TCP/TLS connections. The transport SHALL preserve idle keep-alive connections for at least 10 minutes with per-host capacity of at least 100.

#### Scenario: Requesting upstream with native HTTP/1.1 fingerprint
- **WHEN** an Antigravity request is dispatched to upstream
- **THEN** the transport negotiates HTTP/1.1, clears ALPN protocol advertisement, and reuses the account-scoped connection pool

#### Scenario: Multi-account connection isolation
- **WHEN** requests are dispatched for two different Antigravity accounts
- **THEN** the upstream client uses distinct connection pools and distinct TCP/TLS sessions for each account

### Requirement: Upstream daily endpoint and client version alignment
The Antigravity client SHALL use `https://daily-cloudcode-pa.googleapis.com` as its primary API endpoint in standard forwarding. The default User-Agent version SHALL be at least `2.9.1` formatted as `antigravity/hub/<version> darwin/arm64`.

#### Scenario: Forwarding to primary daily endpoint
- **WHEN** an eligible Antigravity request is initiated without an explicit account override
- **THEN** the request targets `https://daily-cloudcode-pa.googleapis.com` first before attempting fallbacks

#### Scenario: Formatting User-Agent header
- **WHEN** an Antigravity API request is constructed
- **THEN** the `User-Agent` header follows the `antigravity/hub/<version> darwin/arm64` format with version `>= 2.9.1`

### Requirement: Gemini reasoning replay and thought signature restoration
The system SHALL record generated Gemini `thoughtSignature` values in a session-indexed cache during response streaming. When a client submits a subsequent conversation turn with function calls or model turns missing their `thoughtSignature`, the system SHALL restore the matching signature from cache prior to upstream submission.

#### Scenario: Restoring missing signature on tool response
- **WHEN** an incoming multi-turn request contains a function call missing its `thoughtSignature`
- **AND** a matching signature exists in the session replay cache
- **THEN** the gateway injects the cached `thoughtSignature` into the request payload before forwarding to upstream

#### Scenario: Uncached signature fallback
- **WHEN** an incoming request lacks `thoughtSignature` and no cached signature exists
- **THEN** the gateway falls back to `skip_thought_signature_validator`

### Requirement: Strict client-supported Antigravity catalog
The Antigravity catalog and default mapping SHALL exclusively contain models currently supported by the Antigravity client: Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro (and their reasoning effort variants). Deprecated models including `gemini-3-flash`, `gemini-3.1-flash-lite`, `gemini-2.5-*`, and `gemini-3-pro-*` SHALL NOT be published in the active Antigravity catalog or default presets.

#### Scenario: Client model catalog lookup
- **WHEN** a client queries the Antigravity default models
- **THEN** only Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro (with effort variants) are returned, and deprecated models are absent

#### Scenario: Deprecated model rejection from catalog
- **WHEN** inspect the Antigravity model list
- **THEN** `gemini-3-flash`, `gemini-3.1-flash-lite`, and all `gemini-2.5-*` models are absent
