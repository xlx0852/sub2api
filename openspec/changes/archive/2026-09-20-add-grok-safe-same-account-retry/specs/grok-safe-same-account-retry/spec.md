## ADDED Requirements

### Requirement: Exact Grok Account Retry
The gateway SHALL retry an eligible transient Grok HTTP Responses or Chat Completions failure on the exact account used by the failed attempt before selecting another account.

#### Scenario: Transient failure retries the selected account
- **GIVEN** a Grok Responses or Chat request has selected an account
- **AND** the upstream returns a configured transient status before downstream output is committed
- **WHEN** the same-account retry budget is available
- **THEN** the gateway retries the request once using that exact account without invoking account selection for the retry

#### Scenario: Retry exhaustion switches account
- **GIVEN** the exact-account retry has also failed with a failover-eligible error
- **WHEN** another compatible account is available
- **THEN** the gateway applies the existing account health penalty and continues with normal account failover

### Requirement: Conservative Retry Eligibility
The gateway SHALL limit automatic Grok same-account retries to configured transient statuses and SHALL exclude authentication, authorization, request, policy, media, and WebSocket failures.

#### Scenario: Default transient statuses are eligible
- **GIVEN** the default Grok same-account retry configuration
- **WHEN** a pre-output Responses or Chat attempt returns `429`, `502`, `503`, `504`, or `529`
- **THEN** the failure is eligible for one same-account retry

#### Scenario: Deterministic failures are not retried
- **GIVEN** a Grok attempt returns `400`, `401`, `403`, `404`, `409`, or `422`
- **WHEN** the gateway classifies the failure
- **THEN** it does not perform a same-account retry under this policy

#### Scenario: Media and WebSocket requests are excluded
- **GIVEN** a Grok media mutation or WebSocket request fails
- **WHEN** the gateway handles the failure
- **THEN** this same-account retry policy does not retry the request

### Requirement: Retry Output and Deadline Safety
The gateway SHALL NOT retry after downstream output is committed or after the client request context is canceled, and retry delays SHALL be bounded by configuration and the remaining request deadline.

#### Scenario: Downstream output prevents retry
- **GIVEN** a Grok streaming attempt has written any downstream response content
- **WHEN** a later upstream failure occurs
- **THEN** the gateway returns or terminates the current response without retrying or concatenating output from another attempt

#### Scenario: Retry-After is bounded
- **GIVEN** a retryable Grok response includes `Retry-After`
- **WHEN** the advertised delay exceeds the configured maximum or remaining request deadline
- **THEN** the gateway does not wait beyond those bounds

#### Scenario: Client cancellation prevents retry
- **GIVEN** the downstream request context is canceled
- **WHEN** a retryable Grok failure is handled
- **THEN** no same-account retry or account failover request is sent

### Requirement: Retry Observability and Accounting
The gateway SHALL expose structured retry attempt logs while preserving existing successful-attempt usage recording and account concurrency accounting.

#### Scenario: Retry attempt is logged
- **GIVEN** an eligible same-account retry occurs
- **WHEN** the retry begins
- **THEN** the gateway logs the account ID, upstream status, attempt number, maximum attempts, and delay without logging credentials or request content

#### Scenario: Attempt resources are balanced
- **GIVEN** a request performs a same-account retry and then succeeds or fails over
- **WHEN** all attempts finish
- **THEN** each acquired account concurrency slot is released exactly once and usage is recorded only through the existing successful-response path
