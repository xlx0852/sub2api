## ADDED Requirements

### Requirement: Codex Standalone Search Routes
The system SHALL expose Codex standalone alpha/search through `/v1/alpha/search`, `/alpha/search`, and `/backend-api/codex/alpha/search`, and SHALL restrict these routes to OpenAI groups.

#### Scenario: OpenAI group reaches standalone search
- **GIVEN** an authenticated API key belongs to an OpenAI group
- **WHEN** the client posts a valid request to any supported alpha/search route
- **THEN** the gateway dispatches it to the standalone OpenAI search handler

#### Scenario: Non-OpenAI group is rejected
- **GIVEN** an authenticated API key belongs to a non-OpenAI group
- **WHEN** the client posts to an alpha/search route
- **THEN** the gateway returns a not-found response without selecting or contacting an upstream account

### Requirement: Alpha Search Wire Preservation
The system SHALL preserve the evolving alpha/search request and response wire contract, except for explicit model mapping and server-controlled authentication headers.

#### Scenario: Unknown fields remain intact
- **GIVEN** a standalone search request contains supported fields and future unknown fields
- **WHEN** the gateway builds the upstream request
- **THEN** all fields remain intact unless the account model mapping replaces `model`

#### Scenario: Query and response pass through
- **GIVEN** a standalone search request includes query parameters
- **AND** the upstream returns a JSON status and body
- **WHEN** the request completes without failover
- **THEN** the query parameters, response status, content type, allowed headers, and response body are preserved

### Requirement: Account-Type-Specific Search Upstream
The system SHALL route OAuth standalone search to the ChatGPT Codex alpha/search endpoint and API Key standalone search to the validated official or custom OpenAI alpha/search endpoint.

#### Scenario: OAuth search uses Codex backend
- **GIVEN** an eligible OpenAI OAuth account
- **WHEN** standalone search is forwarded
- **THEN** the upstream URL is the ChatGPT Codex alpha/search endpoint
- **AND** the request uses the account access token and ChatGPT account identity headers

#### Scenario: API Key search uses configured base URL
- **GIVEN** an eligible OpenAI API Key account with a custom base URL
- **WHEN** standalone search is forwarded
- **THEN** the gateway validates the base URL and appends the alpha/search endpoint
- **AND** the request uses the account API key

### Requirement: Standalone Search Scheduling and Failover
The system SHALL apply existing billing eligibility, concurrency, account scheduling, health reporting, and HTTP failover policies before returning standalone search results.

#### Scenario: Retryable upstream error switches account
- **GIVEN** the selected OpenAI account returns an error classified as failover-eligible
- **AND** another compatible OpenAI account is available
- **WHEN** no downstream response has been written
- **THEN** the gateway retries the same search request on the next account

#### Scenario: Non-retryable response passes through
- **GIVEN** the selected upstream returns an error that is not failover-eligible
- **WHEN** standalone search completes
- **THEN** the gateway preserves the upstream status and body for the client
