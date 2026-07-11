## ADDED Requirements

### Requirement: Billable requests reserve capacity before upstream execution

The system SHALL atomically reserve balance or subscription capacity before a billable request is sent upstream.

#### Scenario: Concurrent requests exceed available balance

- **WHEN** multiple requests concurrently require more than the user's available balance after existing reservations
- **THEN** only requests covered by available balance are admitted and remaining requests receive a payment-required error before upstream execution

#### Scenario: Upstream request fails

- **WHEN** an admitted request fails, is cancelled, or expires without billable usage
- **THEN** its reservation is released exactly once

#### Scenario: Request completes below reservation

- **WHEN** actual cost is lower than the reserved amount
- **THEN** actual cost is captured and the unused difference is released

### Requirement: Models require pricing before forwarding

The system SHALL reject billable model requests without resolvable pricing before sending them upstream.

#### Scenario: Newly synchronized model has no price

- **WHEN** a model is available from an upstream account but no channel or catalog price resolves for its billing model
- **THEN** the request is rejected with a pricing-unavailable error and no upstream request is made

### Requirement: Billing identity is locally controlled

The system SHALL use a locally generated request or turn identifier as the billing idempotency key.

#### Scenario: Upstream reuses a response identifier

- **WHEN** two distinct WS turns receive the same upstream response identifier
- **THEN** each turn is settled independently while the shared upstream identifier remains available for diagnostics
