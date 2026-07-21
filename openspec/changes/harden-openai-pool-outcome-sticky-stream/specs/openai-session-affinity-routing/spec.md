## ADDED Requirements

### Requirement: OpenAI Scheduling Keeps Three Layers With Stronger Affinity Metadata

The system SHALL preserve the OpenAI account selection order:

1. `previous_response_id` affinity
2. `session_hash` sticky
3. `load_balance`

Sticky and affinity records MUST store at least:

- target `account_id`
- `credential_class`
- `credential_generation`
- `last_used_at`
- `last_reeval_at`

#### Scenario: Generation mismatch invalidates sticky

- **GIVEN** a session sticky entry points at account A with generation G1
- **AND** account A credentials were rotated so current generation is G2
- **WHEN** a later request with the same session hash is scheduled
- **THEN** the sticky entry is treated as invalid
- **AND** selection falls through to a lower layer
- **AND** the invalid entry is not reused

#### Scenario: Credential class mismatch invalidates sticky

- **GIVEN** a sticky entry is stored for credential class `oauth_chatgpt`
- **AND** the current request requires credential class `api_key`
- **WHEN** sticky lookup runs
- **THEN** the entry is not used

### Requirement: Bound Sessions Reevaluate Quota On An Interval

When a valid sticky or previous-response affinity hit occurs, the system SHALL periodically re-evaluate the bound account usage score at a configured interval.

If the bound account usage score is at or above the configured auto-switch threshold and another eligible same-class account has a strictly lower usage score, the system SHALL rebind the affinity to the cooler account for subsequent turns.

Re-evaluation MUST be interval-gated to avoid per-request thrash.

#### Scenario: Hot bound account yields to cooler account after interval

- **GIVEN** thread/session S is bound to account A
- **AND** account A usage score is 90
- **AND** account B is eligible, same credential class, usage score 20
- **AND** the auto-switch threshold is 80
- **AND** the re-eval interval has elapsed since `last_reeval_at`
- **WHEN** a new request for S is scheduled
- **THEN** the scheduler may rebind S to account B
- **AND** the decision is recorded as a sticky re-eval switch

#### Scenario: Re-eval does not flap inside the interval

- **GIVEN** a sticky binding was re-evaluated less recently than the configured interval has not elapsed
- **WHEN** another request arrives for the same sticky key
- **THEN** the scheduler skips quota rebind checks
- **AND** continues using the bound account if still selectable

### Requirement: New Sessions Prefer Lowest Usage Before Weighted Tie-Break

When neither `previous_response_id` nor valid session sticky selects an account, the system SHALL prefer the eligible same-class account with the lowest usage score.

Usage score MUST be computed from the maximum of currently known quota window percents for that account. If no window percent is known, usage score MUST be treated as hottest (100).

If every eligible candidate is unknown, the system SHALL choose by a stable round-robin/deterministic order rather than always pinning one default account.

Existing weighted load-balance factors MAY be used as tie-breakers after usage ordering.

#### Scenario: Fresh session picks the coolest known account

- **GIVEN** no previous_response_id affinity and no valid session sticky
- **AND** account A usage score is 70
- **AND** account B usage score is 15
- **AND** both are eligible and same credential class
- **WHEN** the load-balance layer runs
- **THEN** account B is preferred over account A

#### Scenario: All-unknown pool rotates instead of hard pinning

- **GIVEN** no sticky hit
- **AND** all eligible accounts have unknown usage scores
- **WHEN** multiple new sessions are scheduled
- **THEN** selections are distributed by a stable deterministic rotation/order
- **AND** the system does not permanently pin every new session to a single arbitrary account solely because scores are equal unknowns

### Requirement: previous_response_id Affinity Works On HTTP SSE And WS

The system SHALL resolve and persist `previous_response_id` → account affinity for OpenAI Responses traffic on both WebSocket and HTTP/SSE transports, subject to the same credential class and generation checks.

#### Scenario: HTTP follow-up turn sticks to the prior account

- **GIVEN** a successful HTTP/SSE Responses request on account A produced response id R
- **WHEN** a later HTTP/SSE request includes `previous_response_id=R`
- **AND** account A is still selectable under class/generation checks
- **THEN** scheduling selects account A on the previous_response_id layer

#### Scenario: Expired or invalid response affinity falls through safely

- **GIVEN** a request includes `previous_response_id=R`
- **AND** no live affinity for R exists or generation/class checks fail
- **WHEN** scheduling runs
- **THEN** the gateway falls through to session sticky or load-balance
- **AND** does not error solely because affinity is missing
