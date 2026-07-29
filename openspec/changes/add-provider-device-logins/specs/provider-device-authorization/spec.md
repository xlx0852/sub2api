## ADDED Requirements

### Requirement: Shared Provider Authorization Session
The system SHALL store administrator provider-authorization sessions in Redis and SHALL support cross-instance status reads, cancellation, expiry, and one-time completion consumption.

#### Scenario: Cross-instance authorization
- **WHEN** one instance starts a provider authorization session and another instance receives the status or completion request
- **THEN** the second instance reads the same session and continues the flow safely

#### Scenario: Cancel authorization
- **WHEN** an administrator cancels a pending authorization session
- **THEN** the session cannot be polled, completed, recreated by an in-flight poll, or consumed to create an account

#### Scenario: Consume authorization once
- **WHEN** a successful authorization ticket is consumed
- **THEN** its token result is returned only to the server-side account operation and every later consumption is rejected

### Requirement: Atomic Device Polling
The system SHALL permit at most one active upstream token poll for a device authorization session and SHALL enforce the provider's next allowed poll time.

#### Scenario: Concurrent status requests
- **WHEN** multiple instances query a device session whose next poll time has arrived
- **THEN** one instance acquires the poll lease and other instances return the current pending state without contacting the provider

#### Scenario: Provider slow down
- **WHEN** a provider reports `slow_down`
- **THEN** the session remains pending and its next allowed poll interval is increased before another upstream request

### Requirement: Grok Device Authorization
The system SHALL offer xAI/Grok OAuth device authorization in addition to the existing authorization-code PKCE flow.

#### Scenario: Discover device endpoints
- **WHEN** an administrator starts Grok device authorization
- **THEN** the system discovers the device and token endpoints through xAI OIDC metadata and accepts only HTTPS `x.ai` endpoints

#### Scenario: Complete Grok device login
- **WHEN** xAI authorizes the device code
- **THEN** the system stores the OAuth result behind a one-time ticket, including the discovered token endpoint and parsed account identity

#### Scenario: Existing Grok PKCE login
- **WHEN** an administrator selects the existing Grok browser flow
- **THEN** its authorization-code and refresh-token behavior remains unchanged

### Requirement: OpenAI Codex Device Authorization
The system SHALL offer OpenAI Codex device authorization in addition to every existing OpenAI login and import mode.

#### Scenario: Complete OpenAI device login
- **WHEN** OpenAI returns an authorization code and PKCE values for an approved device request
- **THEN** the system exchanges them through the existing OpenAI OAuth token and account-enrichment pipeline

#### Scenario: Pending OpenAI device request
- **WHEN** OpenAI reports the device request as pending
- **THEN** the session remains pending until its interval or timeout permits the next poll

#### Scenario: Existing OpenAI login modes
- **WHEN** an administrator uses PKCE, refresh token, mobile refresh token, PAT, access token, or Codex session import
- **THEN** the existing workflow remains available and behaviorally unchanged

### Requirement: Reusable Device Authorization Interface
The administrator interface SHALL render provider device authorization from the shared session response without receiving OAuth access or refresh tokens.

#### Scenario: Show device instructions
- **WHEN** a device session starts
- **THEN** the interface displays its verification URL, user code, remaining lifetime, state, and retry timing

#### Scenario: Close device authorization
- **WHEN** the administrator closes or cancels the flow
- **THEN** the interface calls the cancellation operation and stops all local polling
