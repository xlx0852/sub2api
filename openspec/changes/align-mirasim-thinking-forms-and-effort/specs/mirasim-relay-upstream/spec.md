## ADDED Requirements

### Requirement: Relay thinking form follows the signed roster

For a Mirasim relay account the pipeline SHALL normalize the request's thinking controls to the form the
signed roster reports for the final upstream model: `thinking.type=adaptive` with `output_config.effort`
for an adaptive model, and `thinking.type=enabled` with `thinking.budget_tokens` for a model the roster
marks non-adaptive. The roster MUST be read from a local cache only — a cold or unreachable roster MUST NOT
add an upstream call or delay to an inference request, and MUST fall back to the adaptive form. A request
carrying no thinking control MUST be forwarded unchanged.

#### Scenario: Adaptive model receives a fixed token budget
- **WHEN** the request carries `thinking.budget_tokens` and the roster marks the model adaptive
- **THEN** the body SHALL be rewritten to `thinking.type=adaptive` with the nearest `output_config.effort`
- **AND** `thinking.budget_tokens` SHALL be removed

#### Scenario: Non-adaptive model receives an effort level
- **WHEN** the request carries `output_config.effort` and the roster marks the model non-adaptive
- **THEN** the body SHALL carry `thinking.type=enabled` with a `budget_tokens` derived from that level
- **AND** the budget SHALL stay below `max_tokens` and at least 1024

#### Scenario: Cold roster
- **WHEN** no roster entry is cached for the model
- **THEN** the request SHALL use the adaptive form
- **AND** no roster fetch SHALL be issued on the request path

#### Scenario: Request without thinking controls
- **WHEN** the request carries neither `thinking` nor an effort control
- **THEN** the body SHALL be forwarded byte-for-byte unchanged

### Requirement: Relay effort ladder is validated locally

The pipeline SHALL accept only the effort ladder the relay serves for the model family — `low`, `medium`,
`high`, `xhigh`, `max` for Claude/GPT, additionally `off` for DeepSeek (with `low`/`high`/`max` as levels),
and only `low`/`high`/`max` for GLM and Kimi — and SHALL fold the official client's `ultra` into `max`.
An effort outside the model's ladder MUST be rejected locally with HTTP 400 instead of being forwarded.

#### Scenario: DeepSeek receives off
- **WHEN** a DeepSeek model receives `thinking.type=disabled`
- **THEN** the body SHALL drop `thinking` and set `output_config.effort=off`

#### Scenario: GLM receives off
- **WHEN** a GLM or Kimi model receives `thinking.type=disabled`
- **THEN** the request SHALL be rejected with 400 and a message naming the model

#### Scenario: Unsupported level for the family
- **WHEN** a DeepSeek model receives `output_config.effort=medium`
- **THEN** the request SHALL be rejected with 400 and a message naming the model and the ladder

#### Scenario: ultra is folded
- **WHEN** any of `reasoning.effort`, `reasoning_effort`, or `output_config.effort` is `ultra`
- **THEN** the value SHALL be sent as `max`

#### Scenario: Budget form on an adaptive model
- **WHEN** an adaptive model receives `thinking.type=enabled` with a fixed budget
- **THEN** the body SHALL be rewritten to the adaptive form instead of being rejected

### Requirement: Roster fetch never affects the model whitelist

The signed roster SHALL be fetched only on the read-only metadata path and MUST NOT contribute entries to
the account's available-model whitelist, which continues to come solely from the account-dimension
`GET /v1/models`.

#### Scenario: Whitelist after a roster refresh
- **WHEN** the metadata path refreshes the roster for an account
- **THEN** the account's whitelist SHALL still equal the account-dimension `GET /v1/models` response
- **AND** the roster SHALL only have populated the thinking-form cache
