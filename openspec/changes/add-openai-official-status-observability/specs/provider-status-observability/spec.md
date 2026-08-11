## ADDED Requirements

### Requirement: Safe official status collection

The system SHALL collect OpenAI official aggregate status from a fixed allowlisted HTTPS endpoint in a background task that is independent of gateway request processing. The collector MUST enforce bounded polling intervals, request timeouts, response-body limits and redirect-host validation.

#### Scenario: Successful official status poll

- **WHEN** the OpenAI Status endpoint returns a valid summary within configured limits
- **THEN** the system normalizes the aggregate state, component states, active incidents and source timestamps
- **AND** the collection does not execute on a gateway request path

#### Scenario: Unsafe redirect or oversized response

- **WHEN** the status endpoint redirects to a non-allowlisted host or returns a body larger than the configured limit
- **THEN** the system rejects the response and records a collector failure
- **AND** the last known-good official status remains available

### Requirement: Change-only durable status history

The system SHALL persist a provider status snapshot only when normalized official status content changes. The system MUST keep sufficient timestamps and provenance to correlate official state transitions with local Ops metrics.

#### Scenario: Unchanged repeated poll

- **WHEN** consecutive successful polls normalize to the same content hash
- **THEN** the system does not insert duplicate history rows
- **AND** collector heartbeat freshness still advances

#### Scenario: Official incident changes state

- **WHEN** an official incident is created, updated, enters monitoring or resolves
- **THEN** the system persists a new change snapshot containing the new incident state and source update time

### Requirement: Graceful degradation and freshness

The system SHALL expose official status freshness as `fresh`, `stale` or `unavailable`. External collection failures MUST NOT erase the last successful snapshot or alter gateway behavior.

#### Scenario: Status source becomes unavailable

- **WHEN** one or more polls fail after a successful snapshot exists
- **THEN** the system serves the last known-good snapshot with its original source and fetch timestamps
- **AND** marks it stale once the configured threshold is exceeded

#### Scenario: No successful snapshot exists

- **WHEN** the collector has never completed a valid poll
- **THEN** the admin API reports official status as unavailable
- **AND** all gateway forwarding, scheduling, failover, rate limiting and billing continue unchanged

### Requirement: Admin official status APIs

The system SHALL provide authenticated administrator-only APIs for the latest official provider status and bounded status history. Responses MUST include provider, source URL, aggregate status, source update time, fetch time, freshness, components and incidents.

#### Scenario: Administrator reads current status

- **WHEN** an authenticated administrator requests the OpenAI official status
- **THEN** the system returns the latest successful normalized snapshot and collector freshness

#### Scenario: Non-administrator requests status

- **WHEN** a non-administrator calls an official status Ops endpoint
- **THEN** the system denies access using the existing admin authorization behavior

### Requirement: Distinct official and local evidence

The Ops dashboard SHALL display OpenAI official aggregate status separately from Sub2API local request health and SHALL identify the official source and data freshness. Official incident windows MAY be correlated visually with local error trends, but MUST NOT rewrite historical error attribution.

#### Scenario: Official degradation overlaps local errors

- **WHEN** an official incident window overlaps a local error spike
- **THEN** the dashboard shows both signals on the same time axis with distinct labels
- **AND** does not claim causation solely from temporal overlap

#### Scenario: Official status and local metrics disagree

- **WHEN** official status is operational while local errors are elevated, or official status is degraded while local metrics are healthy
- **THEN** the dashboard preserves and displays both states without suppressing either signal

### Requirement: Provider status transition events

The system SHALL create deduplicated Ops observability events when official aggregate state transitions between operational and degraded states. These events MUST be notification-only and MUST NOT trigger account or scheduler mutations.

#### Scenario: Official state enters degradation

- **WHEN** a newly persisted snapshot changes the official aggregate state from operational to a degraded state
- **THEN** the system creates at most one active provider-status event for the transition fingerprint
- **AND** includes provider, official state, incident identifiers and source timestamps

#### Scenario: Official state recovers

- **WHEN** a newly persisted snapshot returns the official aggregate state to operational
- **THEN** the corresponding provider-status event is resolved
- **AND** no account status, schedulability or routing configuration is changed
