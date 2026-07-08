## ADDED Requirements

### Requirement: Grok WebSocket Passthrough Routing
The system SHALL route eligible Grok OAuth Codex Responses WebSocket traffic to a native xAI/Grok WebSocket upstream when Grok WS passthrough is enabled.

#### Scenario: Eligible Grok Codex WS request uses native WS
- **GIVEN** a Grok OAuth account with Grok WS passthrough enabled
- **AND** the downstream request is a Codex Responses WebSocket request
- **WHEN** the request is forwarded
- **THEN** the system sends the turn to the xAI/Grok WebSocket `/responses` upstream
- **AND** it does not force the request through the Grok HTTP bridge first

#### Scenario: Ineligible Grok request keeps HTTP bridge
- **GIVEN** a Grok account with an image, video, non-WebSocket, or explicitly disabled WS request
- **WHEN** the request is forwarded
- **THEN** the system keeps using the existing Grok HTTP bridge or media path

### Requirement: Grok WebSocket Request Compatibility
The system SHALL build Grok WebSocket requests with the OAuth token, Grok CLI-compatible headers, conversation hints, and a WebSocket-compatible body.

#### Scenario: Grok WS request includes required auth and CLI headers
- **GIVEN** an eligible Grok OAuth account
- **WHEN** the system opens the upstream WebSocket
- **THEN** the request includes `Authorization: Bearer <token>`
- **AND** it includes Grok CLI-compatible headers and conversation headers required by the configured Grok base URL

#### Scenario: Grok WS body preserves conversation continuity
- **GIVEN** a downstream Responses WebSocket turn with `previous_response_id`
- **WHEN** the system builds the upstream Grok WS body
- **THEN** it preserves the conversation continuation through the mapped upstream `previous_response_id`
- **AND** it removes HTTP/SSE-only fields that are invalid for the WebSocket upstream

### Requirement: Grok Response ID Mapping
The system SHALL map downstream response IDs to upstream Grok response IDs for multi-turn WebSocket conversations.

#### Scenario: Subsequent turn maps previous response ID
- **GIVEN** the first Grok WS turn returned an upstream response ID
- **AND** the downstream client continues with the downstream response ID
- **WHEN** the next turn is forwarded
- **THEN** the system maps the downstream `previous_response_id` to the correct upstream ID

#### Scenario: Upstream response maps back to downstream ID
- **GIVEN** the upstream Grok WS stream returns response events
- **WHEN** events are forwarded downstream
- **THEN** the system preserves downstream-visible response IDs consistently across the turn

### Requirement: Grok WebSocket Connection Reuse and Eviction
The system SHALL reuse Grok upstream WebSocket connections per account and execution session, and evict failed connections before reuse.

#### Scenario: Same execution session reuses connection
- **GIVEN** an active Grok WS connection for an account and execution session
- **WHEN** a later turn in the same execution session is forwarded
- **THEN** the system reuses the existing connection when it is healthy

#### Scenario: Failed connection is evicted
- **GIVEN** a Grok WS connection fails ping, read, or write
- **WHEN** the next turn attempts to use that connection
- **THEN** the system evicts the failed connection
- **AND** it dials a new connection or falls back according to the configured mode

### Requirement: Grok HTTP Bridge Fallback
The system SHALL preserve Grok HTTP bridge fallback when Grok WS passthrough is disabled, unsupported, or fails in auto mode.

#### Scenario: Auto mode falls back on WS dial failure
- **GIVEN** Grok WS passthrough is set to auto
- **AND** the upstream Grok WS dial fails
- **WHEN** the request can still be served by HTTP bridge
- **THEN** the system forwards the turn through the Grok HTTP bridge
- **AND** it records the fallback reason

#### Scenario: Force mode returns explicit error
- **GIVEN** Grok WS passthrough is set to force
- **AND** the upstream Grok WS path fails before a usable stream is established
- **WHEN** the request is forwarded
- **THEN** the system returns an explicit upstream transport error instead of silently using HTTP bridge

### Requirement: Grok WebSocket Observability
The system SHALL record Grok WS connection reuse, dial, payload, event, preflight, and fallback metrics in the existing account performance pipeline.

#### Scenario: Account performance identifies connection issues
- **GIVEN** Grok WS traffic has been served for an account
- **WHEN** an admin views account performance details
- **THEN** the system exposes whether latency is dominated by connection acquisition, preflight failures, payload size, upstream generation time, or fallback behavior
