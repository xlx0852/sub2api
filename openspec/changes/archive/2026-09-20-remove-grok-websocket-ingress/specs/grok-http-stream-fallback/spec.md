## ADDED Requirements

### Requirement: Grok Uses HTTP Responses Streaming Only

The system SHALL serve Grok text generation through HTTP Responses/SSE and SHALL NOT accept downstream Responses WebSocket ingress for a Grok API-key group.

#### Scenario: Normal Grok streaming request uses SSE
- **GIVEN** an API key assigned to a Grok group
- **WHEN** the client sends `POST /responses` or `POST /v1/responses` with streaming enabled
- **THEN** the system forwards the request through the Grok HTTP Responses path
- **AND** the client receives the normal Responses event stream

#### Scenario: Grok WebSocket upgrade is rejected before handshake
- **GIVEN** an API key assigned to a Grok group
- **WHEN** the client sends a Responses WebSocket upgrade request
- **THEN** the system returns HTTP `426 Upgrade Required` with error type `websocket_not_supported`
- **AND** the system does not send `101 Switching Protocols`
- **AND** it does not allocate WebSocket ingress, user, or account concurrency slots

### Requirement: Misconfigured Client Can Fall Back to HTTP

The system SHALL reject Grok WebSocket ingress in a form compatible with clients that retry failed Responses WebSocket handshakes over HTTPS.

#### Scenario: WebSocket-enabled Codex retries with HTTP
- **GIVEN** a Codex provider incorrectly advertises `supports_websockets = true`
- **AND** the provider API key belongs to a Grok group
- **WHEN** Codex attempts a Responses WebSocket handshake
- **THEN** the handshake is rejected as unsupported
- **AND** Codex retries the turn through the HTTP Responses endpoint
- **AND** the turn completes through the SSE path

### Requirement: OpenAI WebSocket Behavior Is Isolated

The system SHALL preserve existing Responses WebSocket behavior for OpenAI groups.

#### Scenario: OpenAI group still accepts eligible WebSocket ingress
- **GIVEN** an API key assigned to an eligible OpenAI group
- **WHEN** the client sends a valid Responses WebSocket upgrade request
- **THEN** the request continues through the existing OpenAI WebSocket ingress flow
- **AND** the Grok transport guard does not alter its routing or connection lifecycle
