## ADDED Requirements

### Requirement: OpenAI long-context billing is enabled by default

The system SHALL apply model-catalog long-context pricing to OpenAI account usage by default. An account MAY explicitly set `openai_long_context_billing_enabled=false` to disable this policy. Non-OpenAI accounts and models without valid long-context pricing metadata MUST remain unaffected.

#### Scenario: OpenAI account has no explicit setting

- **GIVEN** an OpenAI account does not contain `openai_long_context_billing_enabled`
- **WHEN** the request input usage strictly exceeds the model's long-context threshold
- **THEN** the full request input and input-side cache buckets use the configured input multiplier and the full output uses the configured output multiplier

#### Scenario: OpenAI account explicitly disables long-context billing

- **GIVEN** an OpenAI account contains `openai_long_context_billing_enabled=false`
- **WHEN** its request exceeds the model's long-context threshold
- **THEN** long-context multipliers are not applied

#### Scenario: Request is exactly at the threshold

- **WHEN** mutually exclusive input, cache-write, and cache-read tokens total exactly the configured threshold
- **THEN** long-context multipliers are not applied

#### Scenario: Request exceeds the threshold by one token

- **WHEN** mutually exclusive input, cache-write, and cache-read tokens total one token above the configured threshold
- **THEN** long-context multipliers apply to the full request rather than only the excess token

#### Scenario: Non-OpenAI usage is billed

- **WHEN** a non-OpenAI account is evaluated for long-context billing
- **THEN** the OpenAI account default does not enable or alter that provider's pricing policy
