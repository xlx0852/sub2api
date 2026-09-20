## ADDED Requirements

### Requirement: Scoped simple OpenAI selection profile
The system SHALL enable simple selection only for explicitly configured OpenAI group IDs and non-compact requests. Other providers and compact selection MUST retain their existing ordering.

#### Scenario: Default deployment and rollback
- **WHEN** the simple selection group list is empty
- **THEN** the existing scheduling profile is used without changing configured score weights

#### Scenario: Other platform shares the gateway service
- **WHEN** Grok, Kimi or a compact request uses the same service instance
- **THEN** the simple OpenAI profile does not alter its selection order

### Requirement: Invalid preferences do not constrain the candidate pool
The system SHALL use the same hard eligibility checks for ordinary affinity, load balancing, and final candidate rechecks. A historical sticky or user pin MUST NOT impose a credential-class filter on new independent requests. Invalid bindings SHALL be ignored or removed.

#### Scenario: Disabled apikey account previously served a session
- **WHEN** a session refers to a disabled apikey account that still has the requested model mapping
- **AND** an OAuth account in the same group satisfies all hard gates
- **THEN** the OAuth account participates in selection rather than being rejected by the old binding's credential class

#### Scenario: Preferred account has no free slot
- **WHEN** a valid ordinary preference cannot acquire an account slot
- **THEN** other eligible candidates are attempted before a bounded wait plan is returned

### Requirement: Simple ordering uses priority and live load
The system SHALL order eligible candidates by administrator priority, then available concurrent load rate, and randomize only ties using an injectable random source. It MUST NOT use historical performance, unknown quota, pool tickets or TopK truncation to exclude otherwise eligible candidates in this profile.

#### Scenario: New account has no history
- **WHEN** a new account and a used account have the same priority and live load
- **THEN** both can become the first candidate independently of TTFT/error/quota history

#### Scenario: All first-tier slots are busy
- **WHEN** the highest-priority accounts fail slot acquisition
- **THEN** remaining eligible accounts, including lower-priority candidates, are attempted

### Requirement: Explicit continuation and authentication constraints survive simplification
The system SHALL preserve non-movable previous-response affinity and request-local credential class restrictions after an upstream authentication failure. It MUST NOT silently strip continuation state or retry a credential failure in a different credential class.

#### Scenario: Bound continuation cannot be moved
- **WHEN** a previous-response request requires an unavailable original account and is not marked safely movable
- **THEN** selection reports continuation unavailability without selecting an unrelated account or acquiring its slot

#### Scenario: OAuth authentication fails during this request
- **WHEN** an OAuth attempt returns 401 or 403
- **THEN** subsequent selection within that request excludes apikey accounts even if the account has ignore_upstream_401 enabled
- **AND** a new independent request does not inherit this restriction
