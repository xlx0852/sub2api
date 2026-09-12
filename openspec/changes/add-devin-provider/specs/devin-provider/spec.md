## ADDED Requirements

### Requirement: Devin Account Transport

The system SHALL support a `devin` platform account whose token and optional
base URL/model credentials are used to call Devin's Connect
`ApiServerService/GetChatMessage` stream.

#### Scenario: Devin account request

- **WHEN** a scheduled Devin account receives a supported gateway request
- **THEN** the gateway sends a protobuf Connect request with the full
  conversation and authenticates it with the configured Devin session token

#### Scenario: Missing Devin token

- **WHEN** a Devin account has no usable token in `token`, `access_token`, or
  `api_key`
- **THEN** the gateway returns an authentication error without attempting an
  upstream request

### Requirement: Devin Protocol Compatibility

The system SHALL expose Devin responses through the existing OpenAI Responses,
OpenAI Chat Completions, and Anthropic Messages gateway formats, including
streaming text, reasoning, tool calls, and usage when Devin supplies them.

#### Scenario: Streaming response

- **WHEN** Devin emits multiple response frames for a streaming request
- **THEN** the gateway emits ordered client protocol events and a terminal
  completion event without dropping later usage frames

#### Scenario: Tool call response

- **WHEN** Devin emits fragmented tool-call arguments
- **THEN** the gateway accumulates the fragments and exposes one complete tool
  call with the original call id and name

### Requirement: Devin Model Discovery

The system SHALL allow Devin account model discovery through the existing
admin/account model path and SHALL retain an explicitly configured model even
when the upstream model directory is unavailable.

#### Scenario: Configured model fallback

- **WHEN** the Devin model directory cannot be fetched but the account has a
  configured model
- **THEN** the account remains callable for that model and the failure is
  reported only to the discovery caller
