## ADDED Requirements

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
