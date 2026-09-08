## ADDED Requirements

### Requirement: Cursor Client Fingerprint and Checksum
The system SHALL generate authentic `x-cursor-checksum` tokens using the obfuscated timestamp and machine identifier, and inject required client fingerprint headers for all upstream requests to Cursor / Sand backends.

#### Scenario: Valid checksum generation
- **WHEN** an upstream request is prepared for account with machine identifier `M` at time `T`
- **THEN** the system generates an `x-cursor-checksum` header combining base64url-encoded obfuscated timestamp bytes and `M`

#### Scenario: Client fingerprint headers
- **WHEN** the system establishes a Connect-RPC stream to `api2.cursor.sh`
- **THEN** it sets `x-cursor-client-type: sand`, `x-cursor-client-version: 0.27.0`, `x-sand-box-namespace: prod`, `x-ghost-mode: false`, and a unique `x-request-id`

### Requirement: Cursor PKCE Authorization
The system SHALL provide a browser PKCE authorization workflow that allows administrators to authorize Cursor accounts and securely capture tokens behind short-lived login tickets.

#### Scenario: Start PKCE authorization
- **WHEN** an administrator starts Cursor authorization with an optional proxy
- **THEN** the system returns a browser authorization URL with PKCE challenge and a polling session identifier

#### Scenario: Complete PKCE authorization
- **WHEN** upstream confirms user completed authorization in browser
- **THEN** the system stores the token result behind a short-lived one-time ticket and marks session successful

#### Scenario: Create account with ticket
- **WHEN** an administrator submits valid account settings with an unused successful Cursor login ticket
- **THEN** the system creates `platform=cursor`, `type=oauth` with access token, refresh token, and machine ID, invalidating the ticket

### Requirement: Cursor Token Lifecycle and Refresh
The system SHALL refresh expiring Cursor OAuth tokens through the shared distributed refresh framework and respect account proxy settings.

#### Scenario: Refresh before expiry
- **WHEN** a schedulable Cursor OAuth account approaches token expiration
- **THEN** the system refreshes the credentials using the refresh token and updates the account and cache

#### Scenario: Rotated refresh token
- **WHEN** the upstream returns a new refresh token during refresh
- **THEN** the system atomically updates both access and refresh tokens

### Requirement: Cursor Connect-RPC Gateway Forwarding
The system SHALL translate downstream Chat Completions requests into Cursor Connect-RPC `InferenceService.Stream` protocol, streaming back OpenAI-compatible chunks while preserving thinking, tool calls, and usage metrics.

#### Scenario: Non-streaming chat completion
- **WHEN** a client submits a non-streaming Chat Completions request for a Cursor account
- **THEN** the system aggregates text, thinking, and tool call parts from the stream and returns a single `chat.completion` response

#### Scenario: Streaming chat completion
- **WHEN** a client submits a streaming request with `stream: true`
- **THEN** the system emits real-time SSE `chat.completion.chunk` events and terminates with `data: [DONE]`

#### Scenario: Function calling and tool accumulation
- **WHEN** upstream streams fragmented `toolCallPart` deltas
- **THEN** the system accumulates tool call arguments and emits complete tool calls to downstream clients

### Requirement: Cursor Quota and Profile Sync
The system SHALL query the Cursor Dashboard service to synchronize user profile, Fast Requests balance, and Sand usage percent.

#### Scenario: Periodic quota check
- **WHEN** the system synchronizes quota for a Cursor account
- **THEN** it queries `DashboardService` for user profile and current usage status
