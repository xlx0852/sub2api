## ADDED Requirements
### Requirement: Antigravity OpenAI Responses adapter
The gateway SHALL accept OpenAI Responses requests for Antigravity accounts and translate them to supported text Gemini Antigravity requests. Existing provider routing SHALL remain unchanged.

#### Scenario: Ordinary non-streaming Responses turn
- **WHEN** an authenticated request targets `/v1/responses` with a text input and an Antigravity account is selected
- **THEN** the gateway sends a Gemini text request upstream and returns an OpenAI Responses JSON response

#### Scenario: Ordinary streaming Responses turn
- **WHEN** an authenticated request targets `/v1/responses` with `stream=true` and an Antigravity account is selected
- **THEN** the gateway emits valid OpenAI Responses SSE events while reading the Antigravity Gemini stream

#### Scenario: Previous response continuation
- **WHEN** a request includes a valid `previous_response_id`
- **THEN** the adapter restores owned, model-scoped Gemini history from bounded process-local storage and does not silently discard it

#### Scenario: Expired or foreign continuation
- **WHEN** previous_response_id is missing from the process-local cache, expired, or belongs to a different API Key, group, account, or model
- **THEN** the adapter rejects the request and directs the caller to replay full input history

#### Scenario: Stateless client
- **WHEN** the caller uses store=false and supplies full input history
- **THEN** the adapter forwards that history without storing a continuation record

#### Scenario: Function call output continuation
- **WHEN** a request includes `function_call_output`
- **THEN** the adapter converts it into the corresponding Gemini function response content and returns the next OpenAI Responses turn

#### Scenario: Unsupported Antigravity model
- **WHEN** the request resolves to a Claude or image Antigravity model
- **THEN** the adapter rejects it before upstream execution with an explicit unsupported-model error
