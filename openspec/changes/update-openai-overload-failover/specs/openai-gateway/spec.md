## ADDED Requirements

### Requirement: Safe cross-account recovery for upstream overload

The OpenAI gateway SHALL retry a retryable upstream overload or service-unavailable failure on an untried account from the same eligible group when no visible response content has been sent to the client.

#### Scenario: Early overload is recovered by another account

- **WHEN** an eligible OpenAI account returns a retryable overload before visible output is sent
- **THEN** the gateway selects an untried eligible account in the same group, applies bounded jittered backoff, and continues the request without exposing a 503 if a later attempt succeeds

#### Scenario: All eligible accounts have been attempted

- **WHEN** every eligible same-group account has failed with a retryable overload
- **THEN** the gateway stops after the configured attempt limit and returns one terminal error without repeatedly selecting the same account

### Requirement: Stream replay safety and observability

The OpenAI gateway MUST NOT replay a request after visible response content has been sent, and MUST record attempt order, account identity, failure stage, upstream status, and final client outcome without logging credentials or content.

#### Scenario: Terminal failure after visible output

- **WHEN** the upstream emits a terminal failure after visible output has reached the client
- **THEN** the gateway does not replay the request, preserves billing safety, and records the terminal failure stage

#### Scenario: Retryable failure before visible output

- **WHEN** a retryable failure occurs before visible output
- **THEN** the gateway records the failed account and retry decision, while keeping request content and credentials out of logs
