## ADDED Requirements

### Requirement: Grok API Key Account Management
The system SHALL allow administrators to create and update Grok accounts authenticated by an xAI-compatible API key.

#### Scenario: Create an official xAI API Key account
- **GIVEN** an administrator selects the Grok platform and API Key account type
- **WHEN** a valid API key is submitted without a custom base URL
- **THEN** the account stores the API key and uses `https://api.x.ai/v1`
- **AND** official API routing is selected explicitly

#### Scenario: Preserve Grok OAuth behavior
- **GIVEN** an existing Grok OAuth account
- **WHEN** the API Key capability is enabled
- **THEN** its OAuth refresh, CLI text routing, quota probing, and subscription display remain unchanged

### Requirement: Grok API Key Model Discovery
The system SHALL discover models for a Grok API Key account from its xAI-compatible model-list endpoint.

#### Scenario: Synchronize official xAI models
- **GIVEN** a Grok API Key account with a valid credential
- **WHEN** the administrator synchronizes upstream models
- **THEN** the system requests `/v1/models` with Bearer authentication
- **AND** returns the deduplicated upstream model IDs

#### Scenario: Upstream does not expose model discovery
- **GIVEN** a configured Grok-compatible upstream without a working `/v1/models` endpoint
- **WHEN** model synchronization fails
- **THEN** the system returns a sanitized synchronization error
- **AND** the administrator can retain a manually configured model whitelist

### Requirement: Grok API Key Connection Testing
The system SHALL test Grok API Key accounts against the configured Responses API using the stored API key.

#### Scenario: API Key account test succeeds
- **GIVEN** a Grok API Key account with a valid model and credential
- **WHEN** the administrator starts a connection test
- **THEN** the system sends a streaming request to `/v1/responses`
- **AND** reports the returned response events through the existing test stream

### Requirement: Grok API Key Gateway Forwarding
The system SHALL forward Grok API Key traffic through the existing official xAI-compatible routes.

#### Scenario: Text and media requests use API Key authentication
- **GIVEN** a schedulable Grok API Key account
- **WHEN** a Responses, Chat Completions, image, or video request is routed to it
- **THEN** the system authenticates with the stored API key
- **AND** does not attach Grok CLI chat-proxy identity headers
