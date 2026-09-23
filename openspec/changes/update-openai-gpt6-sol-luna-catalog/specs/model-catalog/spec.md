## ADDED Requirements

### Requirement: GPT-6 Sol and Luna catalog availability

The system SHALL expose `gpt-6-sol` and `gpt-6-luna` as OpenAI chat models in the first-party
model catalog, keep their requested IDs when routing, and publish them as selectable presets.

#### Scenario: Models appear in the catalog

- **WHEN** the first-party model catalog is loaded
- **THEN** the OpenAI platform model list contains `gpt-6-sol` and `gpt-6-luna` without changing any existing entry

#### Scenario: Models are selectable in the client

- **WHEN** the frontend builds the OpenAI model presets
- **THEN** `gpt-6-sol` and `gpt-6-luna` are available as selectable presets

### Requirement: GPT-6 Sol and Luna pricing resolution

The system SHALL resolve official OpenAI pricing for `gpt-6-sol` and `gpt-6-luna` instead of
inheriting the default test model price, and SHALL apply the official long-context tier above
272,000 input tokens.

#### Scenario: Sol pricing resolution

- **WHEN** `gpt-6-sol` is queried for pricing
- **THEN** the input cost is 2 USD per million tokens, the output cost is 10 USD per million tokens, the cache read cost is 0.2 USD per million tokens, and the cache write cost is 2.5 USD per million tokens

#### Scenario: Luna pricing resolution

- **WHEN** `gpt-6-luna` is queried for pricing
- **THEN** the input cost is 0.1 USD per million tokens, the output cost is 0.5 USD per million tokens, the cache read cost is 0.01 USD per million tokens, and the cache write cost is 0.125 USD per million tokens

#### Scenario: Long context tier applies

- **WHEN** a GPT-6 Sol or Luna request exceeds 272,000 total input tokens
- **THEN** the whole request is billed with a 2x input multiplier and a 1.5x output multiplier

#### Scenario: Date-suffixed variants inherit the family price

- **WHEN** a model ID such as `gpt-6-sol-2026-10-01` is queried for pricing
- **THEN** it resolves to the `gpt-6-sol` family pricing rather than the default test model price

### Requirement: Daybreak cyber model pricing

The system SHALL provide official OpenAI pricing rows for the Daybreak cyber models
`gpt-5.6-cyber` and `gpt-5.5-cyber`.

#### Scenario: Cyber model pricing resolution

- **WHEN** `gpt-5.6-cyber` is queried for pricing
- **THEN** the input cost is 12.5 USD per million tokens, the output cost is 75 USD per million tokens, the cache read cost is 1.25 USD per million tokens, and the cache write cost is 15.625 USD per million tokens

### Requirement: GPT Pro pool exposure

The GPT Pro group SHALL expose `gpt-6-sol` and `gpt-6-luna` through an account whose upstream
advertises support, so the models are routable and billable end to end.

#### Scenario: Group model list includes the models

- **WHEN** a GPT Pro API key requests `/v1/models`
- **THEN** the response contains `gpt-6-sol` and `gpt-6-luna`

#### Scenario: Request is routed rather than rejected

- **WHEN** a GPT Pro API key sends a chat or Responses request for `gpt-6-sol`
- **THEN** the gateway selects an eligible account instead of failing model resolution
