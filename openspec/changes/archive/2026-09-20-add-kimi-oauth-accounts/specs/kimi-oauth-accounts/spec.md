## ADDED Requirements

### Requirement: Kimi Device Authorization
The system SHALL allow an administrator to authenticate a Kimi account using the OAuth 2.0 Device Authorization Grant without exposing the resulting refresh token to the browser.

#### Scenario: Start device authorization
- **WHEN** an administrator starts Kimi OAuth authorization with an optional account proxy
- **THEN** the system returns a verification URL, user code, expiry, polling interval, and opaque session identifier

#### Scenario: Complete device authorization
- **WHEN** Kimi reports that the user authorized a pending device session
- **THEN** the system stores the token result behind a short-lived one-time login ticket and reports success without returning access or refresh tokens

#### Scenario: Pending and slow authorization
- **WHEN** Kimi reports `authorization_pending` or `slow_down`
- **THEN** the system keeps the session pending and enforces the upstream polling interval, increasing it after `slow_down`

#### Scenario: Concurrent status polling
- **WHEN** multiple instances request status after the Kimi polling interval elapses
- **THEN** one instance polls Kimi while the others return the current pending state without duplicating the upstream request

#### Scenario: Cancel or expire authorization
- **WHEN** an administrator cancels a device session or its device code expires
- **THEN** the system removes its sensitive state and cannot create an account from that session

#### Scenario: Cancel while polling
- **WHEN** an administrator cancels a Kimi device session while an upstream token poll is in flight
- **THEN** the completed poll cannot recreate the session or persist its token result

### Requirement: Kimi OAuth Account Creation
The system SHALL create a first-class Kimi OAuth account by consuming a successful one-time login ticket and SHALL persist the token expiry, scope, refresh token, and stable device identifier.

#### Scenario: Consume successful ticket
- **WHEN** an administrator submits valid account settings with an unused successful Kimi login ticket
- **THEN** the system creates `platform=kimi`, `type=oauth` and permanently invalidates the ticket

#### Scenario: Reject replayed ticket
- **WHEN** a consumed, expired, or mismatched Kimi login ticket is submitted
- **THEN** the system rejects account creation without disclosing credential contents

### Requirement: Kimi Token Lifecycle
The system SHALL refresh expiring Kimi OAuth credentials through the shared distributed refresh framework and SHALL use the account proxy and stored device identity for every refresh.

#### Scenario: Refresh before expiry
- **WHEN** a schedulable Kimi OAuth account approaches token expiry
- **THEN** one worker refreshes it and updates the account, token cache, and scheduler cache

#### Scenario: Rotated refresh token
- **WHEN** Kimi returns a new refresh token
- **THEN** the system atomically replaces the previous refresh token while preserving unrelated credentials

#### Scenario: Concurrent refresh
- **WHEN** multiple requests or instances attempt to refresh the same Kimi account
- **THEN** the distributed refresh lock and database reread prevent reuse of a stale refresh token

### Requirement: Kimi Coding Gateway
The system SHALL route Kimi OAuth account traffic to the Kimi Coding API using the protocol supported by each inbound endpoint.

#### Scenario: Chat Completions request
- **WHEN** a Chat Completions request is scheduled to a Kimi OAuth account
- **THEN** it is forwarded to Kimi Coding `/v1/chat/completions` with Kimi model and device metadata

#### Scenario: Anthropic Messages request
- **WHEN** an Anthropic Messages request is scheduled to a Kimi OAuth account
- **THEN** it is converted and forwarded to Kimi Coding `/v1/chat/completions` and the downstream response retains Anthropic semantics

#### Scenario: Responses request
- **WHEN** an OpenAI Responses request is scheduled to a Kimi OAuth account
- **THEN** it is converted to Kimi Chat Completions rather than sent to a nonexistent Kimi Responses endpoint

#### Scenario: Kimi model alias
- **WHEN** a public model contains the `kimi-` prefix, `[1m]` marker, or thinking suffix
- **THEN** the upstream model contains the Kimi canonical model identifier while preserving the supported thinking suffix

### Requirement: Kimi Tool and Thinking Compatibility
The system SHALL normalize Kimi tool-message links and thinking fields only on Kimi account traffic.

#### Scenario: Recover an unambiguous tool result link
- **WHEN** a Kimi tool result lacks `tool_call_id` and exactly one pending assistant tool call can be identified
- **THEN** the system supplies that identifier before forwarding

#### Scenario: Preserve ambiguous tool result
- **WHEN** a missing tool result identifier has multiple possible tool calls
- **THEN** the system does not guess an identifier and records a redacted compatibility warning

#### Scenario: Streaming usage
- **WHEN** a streaming Kimi request is forwarded
- **THEN** upstream usage is requested and translated into the existing local accounting fields

### Requirement: Kimi Administrator Experience
The system SHALL expose Kimi consistently in administrator platform selectors and SHALL provide a reusable device authorization interface.

#### Scenario: Device login display
- **WHEN** a Kimi device session starts
- **THEN** the interface shows the verification link, user code, remaining lifetime, and current authorization state

#### Scenario: Existing platform behavior
- **WHEN** administrators create or manage non-Kimi accounts
- **THEN** their existing authorization-code and API-key workflows remain unchanged
