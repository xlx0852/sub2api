## ADDED Requirements

### Requirement: Completed usage survives process termination before billing

The system SHALL durably persist a versioned settlement payload after terminal upstream usage is completely extracted and before asynchronous billing becomes the sole owner of that work.

#### Scenario: Process exits after terminal usage is extracted

- **WHEN** a billable request has complete terminal usage and the gateway process exits before applying the charge
- **THEN** a restarted instance recovers the original settlement payload and applies it exactly once

#### Scenario: In-memory worker submission is lost

- **WHEN** the in-memory usage worker is stopped, full, or destroyed after a durable settlement payload exists
- **THEN** the database-backed recovery worker can claim and complete the settlement without relying on the original process

### Requirement: Billing effects and usage evidence commit atomically

The system SHALL commit the billing idempotency record, customer monetary effect, applicable quota effects, corresponding usage log, and settled event state in one PostgreSQL transaction.

#### Scenario: Process exits before transaction commit

- **WHEN** the process exits after executing one or more billing statements but before the transaction commits
- **THEN** none of those statements remain visible and the same durable settlement can be retried

#### Scenario: Process exits after transaction commit but before acknowledgement

- **WHEN** the transaction commits and the worker exits before acknowledging completion
- **THEN** a retry observes the existing billing identity, produces no second charge, and converges the event to settled

#### Scenario: Usage log insertion fails

- **WHEN** the usage log cannot be inserted in the billing transaction
- **THEN** the monetary and quota effects are rolled back with the settlement state

### Requirement: Upstream attempts without complete usage remain auditable

The system SHALL persist a stable local billing identity for a request that reaches an upstream provider and SHALL NOT silently classify that request as settled when complete terminal usage is unavailable.

#### Scenario: Process exits before terminal usage

- **WHEN** an upstream request may have produced provider cost but the gateway exits before complete terminal usage is persisted
- **THEN** the attempt remains recoverable or transitions to an explicit reconciliation-required state

#### Scenario: Provider cannot return historical usage

- **WHEN** complete usage cannot be recovered from a provider before the reconciliation deadline
- **THEN** the system retains an auditable reconciliation-required record and does not invent partial or zero usage as a successful settlement

### Requirement: Settlement retries are safe across instances

The system SHALL use the existing locally controlled request or turn identity and request fingerprint to make settlement claim and application idempotent across concurrent instances and restarts.

#### Scenario: Two workers claim the same settlement

- **WHEN** two gateway instances attempt to process the same ready settlement
- **THEN** database claim and billing idempotency permit at most one monetary application

#### Scenario: Reused identity has a different request fingerprint

- **WHEN** a settlement identity already exists with a different request fingerprint
- **THEN** processing stops with an auditable conflict and no additional charge is applied

### Requirement: Durability changes preserve gateway behavior

The system SHALL preserve existing protocol input/output, usage extraction, pricing, multiplier, scheduling, configuration-key, and default deployment semantics while changing settlement persistence.

#### Scenario: Old and new implementations receive the same deterministic upstream response

- **WHEN** old and new implementations process identical HTTP, SSE, or WS requests with identical upstream events
- **THEN** their client-visible responses and calculated usage/cost fields are equal before persistence-only metadata is considered

### Requirement: Crash recovery is a release gate

The system SHALL validate settlement durability with real child-process termination and isolated database instances before enabling the durable path for production traffic.

#### Scenario: Crash injection at every persistence boundary

- **WHEN** the test harness terminates the gateway after terminal extraction, after durable event persistence, before billing commit, and after commit before acknowledgement
- **THEN** restart recovery converges to exactly one charge and exactly one corresponding usage log for every settlement with complete usage

### Requirement: Authoritative rollout is explicitly scoped

The system SHALL require an explicit positive group scope before enabling shadow or authoritative settlement and SHALL keep unselected groups on the existing settlement path.

#### Scenario: A test group is selected for authoritative settlement

- **WHEN** authoritative mode starts with an explicit list of group IDs
- **THEN** matching API keys use durable handoff and durable claims while non-matching API keys create no settlement journal row and retain the existing worker-pool billing path

#### Scenario: An active mode has no valid rollout scope

- **WHEN** shadow or authoritative mode starts with a missing, empty, malformed, or non-positive group scope
- **THEN** the instance fails closed to off and does not take durable billing authority

#### Scenario: A shadow group later becomes authoritative

- **WHEN** a group with existing shadow journal rows is restarted in authoritative mode
- **THEN** the worker claims only attempts created with authoritative eligibility and never charges the historical shadow rows
