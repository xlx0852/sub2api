## ADDED Requirements

### Requirement: Upstream Outcomes Are Classified Before Health Updates

The system SHALL map every finished OpenAI upstream attempt to exactly one outcome class before mutating account health, cooldown, or sticky state:

- `success` for completed 2xx attempts
- `caller` for client/request 4xx errors other than auth and quota
- `credential` for 401/403 and explicit authentication failures
- `quota` for 429 and explicit quota exhaustion
- `transient` for 5xx, connect failures, timeouts, and stream stall terminations attributed to upstream silence

#### Scenario: Caller 4xx does not degrade account health

- **GIVEN** an upstream OpenAI attempt returns HTTP 400 with a request validation error
- **WHEN** the gateway records the attempt outcome
- **THEN** the outcome class is `caller`
- **AND** the account consecutive failure counter is not incremented
- **AND** existing sticky bindings for the account remain intact

#### Scenario: Transient 5xx increments failure health

- **GIVEN** an upstream OpenAI attempt returns HTTP 503
- **WHEN** the gateway records the attempt outcome
- **THEN** the outcome class is `transient`
- **AND** the account consecutive failure counter is updated under the transient sliding window

### Requirement: Quota Cooldown Prefers Retry-After And Reset Hints

For `quota` outcomes the system SHALL compute `cooldownUntil` using this precedence:

1. `Retry-After` header (delta-seconds or HTTP-date)
2. Codex/OpenAI reset window headers already understood by the gateway
3. configured default quota cooldown

The resulting delay MUST be clamped to configured minimum and maximum bounds.

#### Scenario: Retry-After wins over default cooldown

- **GIVEN** an upstream response is HTTP 429 with `Retry-After: 120`
- **AND** no shorter authoritative reset hint overrides it under the precedence rules
- **WHEN** the gateway applies quota handling
- **THEN** the account or key enters cooldown of approximately 120 seconds
- **AND** the outcome class is `quota`

#### Scenario: Success does not clear an active cooldown

- **GIVEN** an account is still inside a quota cooldown window
- **WHEN** a later attempt on that account returns 2xx success
- **THEN** the outcome class is `success`
- **AND** consecutive failures may be cleared
- **AND** the remaining cooldownUntil is preserved until it expires

### Requirement: Credential Failures Fail Closed For Sticky And Credential Class

On `credential` outcomes the system SHALL:

- mark the account as needing reauthentication and/or temporarily/permanently unschedulable per existing severity rules
- clear sticky bindings that point at that account
- prevent failover within the same request from selecting a different credential class than the failed attempt

OAuth/ChatGPT-login accounts and API-key accounts are distinct credential classes and MUST NOT silently fall through into each other during scheduling or failover.

#### Scenario: 401 clears sticky and stays inside OAuth class

- **GIVEN** a request was scheduled onto an OAuth OpenAI account via session sticky
- **AND** upstream returns HTTP 401
- **WHEN** the gateway handles the credential failure and attempts failover
- **THEN** sticky entries for that account are cleared
- **AND** any failover candidate set contains only OAuth credential-class accounts
- **AND** API-key accounts in the same group are not selected for that failover chain

#### Scenario: API-key failure does not fall through to OAuth pool

- **GIVEN** a request was scheduled onto an API-key OpenAI account
- **AND** upstream returns HTTP 403 classified as credential failure
- **WHEN** failover runs
- **THEN** OAuth/ChatGPT-login accounts are excluded from the candidate set for that request

### Requirement: API Key Pool Cooldown Uses Attempted Key CAS Rotate

When an account uses multiple API keys, a `quota` outcome on one key SHALL cool down the attempted key identity and MAY rotate to another non-cooled key using compare-and-set semantics against the live key pointer.

#### Scenario: Concurrent 429 does not cool the replacement key

- **GIVEN** two in-flight requests used key A
- **AND** both receive HTTP 429
- **AND** the first handler rotates live key from A to B
- **WHEN** the second handler applies key failover
- **THEN** key A is cooled
- **AND** key B is not cooled solely because of the stale attempted-key failure
- **AND** the second handler does not perform a redundant rotate away from healthy key B without a new failure on B
