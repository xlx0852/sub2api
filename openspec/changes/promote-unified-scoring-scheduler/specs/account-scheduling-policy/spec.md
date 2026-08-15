## ADDED Requirements

### Requirement: Hard Candidate Policies Precede Scoring

The unified scheduler SHALL evaluate named hard candidate policies before affinity, scoring, TopK, or weighted exploration. A candidate rejected by any hard policy MUST NOT be selected regardless of score.

Hard policies SHALL include the applicable platform/group, schedulable/model, exclusion, credential class, transport/capability, quota pause/cooldown, RPM/window-cost, and mixed-platform constraints supplied by platform adapters.

#### Scenario: High score cannot bypass quota cooldown

- **GIVEN** an account has the highest computed score
- **AND** its quota cooldown policy rejects it
- **WHEN** unified selection runs
- **THEN** the account is absent from the scored candidate set
- **AND** it is not selected

#### Scenario: First rejected policy is diagnosable

- **GIVEN** an account fails more than one declared hard policy
- **WHEN** policies run in their configured order
- **THEN** the decision records the first rejected policy name
- **AND** replaying the same snapshot yields the same rejection

### Requirement: Shared Scoring Uses Immutable Available Signals

The scheduler SHALL score immutable candidate snapshots using platform-independent signal types for priority, load, queue depth, error rate, TTFT, quota headroom, reset horizon, compact health, and affinity.

Every optional signal MUST carry availability/freshness state. Missing or stale signals MUST NOT silently behave as a favorable zero measurement.

#### Scenario: Gateway statistics are not warmed

- **GIVEN** a Gateway candidate has no fresh TTFT or error sample
- **WHEN** its snapshot is built
- **THEN** those signals are marked unavailable
- **AND** the scorer does not grant an advantage equivalent to measured zero error and zero TTFT

#### Scenario: OpenAI formula migrates without drift

- **GIVEN** a fixed OpenAI candidate/load/runtime snapshot, weights, clock, and random seed
- **WHEN** the legacy scorer and shared scorer run
- **THEN** their normalized signals, score breakdown, TopK set, weighted order, and selected account are equal

### Requirement: Affinity And Exploration Are Explicit Decisions

The unified scheduler SHALL model previous-response affinity, session affinity, credential/generation validity, re-evaluation, and sticky escape as explicit decisions before producing a TopK weighted order.

Platform rollout policy SHALL determine whether a valid affinity is hard, weighted, or allowed to escape. `off` and `shadow` modes MUST NOT change the live platform's existing affinity behavior.

#### Scenario: Shadow sticky escape differs without changing the request

- **GIVEN** the legacy Gateway path keeps a valid sticky account
- **AND** the shadow scorer would escape it due to approved runtime thresholds
- **WHEN** shadow evaluation runs
- **THEN** the legacy sticky account remains the only live selection
- **AND** the counterfactual escape reason and candidate are recorded without writing sticky state

#### Scenario: Weighted TopK is replayable in tests

- **GIVEN** fixed candidate snapshots, weights, TopK, clock, and random seed
- **WHEN** weighted exploration runs repeatedly
- **THEN** it produces the same ordered candidates and final selection

### Requirement: Runtime Feedback Is Bounded And Non-Sensitive

Runtime scheduling statistics SHALL be bounded by TTL and capacity and SHALL contain only account-level operational aggregates needed for scheduling. They MUST NOT contain request content, response content, credentials, bearer tokens, or raw session identifiers.

#### Scenario: Expired account statistics stop influencing score

- **GIVEN** an account's runtime sample is older than the configured freshness window
- **WHEN** a new candidate snapshot is built
- **THEN** the expired signals are marked unavailable
- **AND** the stale values do not affect selection

#### Scenario: Account churn does not grow stats without bound

- **GIVEN** accounts are deleted or have not produced feedback beyond the TTL
- **WHEN** stats cleanup runs
- **THEN** their entries are removed
- **AND** the store remains within its configured capacity

### Requirement: Scoring Rollout Is Shadowable And Reversible

The system SHALL support database-controlled `off`, `shadow`, and `enforce` modes scoped by platform and group, with `off` as the default. Switching to `off` MUST stop scoring from affecting live selection without a process restart.

In `shadow`, the legacy selector SHALL be the only component allowed to acquire concurrency, write affinity, register sessions, or create wait plans. The shared scorer SHALL produce only a counterfactual decision.

#### Scenario: Shadow has no scheduling side effects

- **GIVEN** scoring mode is `shadow` for a group
- **WHEN** legacy and shared selection are evaluated
- **THEN** exactly one live account acquire/release lifecycle occurs through the legacy path
- **AND** shared evaluation does not write sticky state, register a session, or enter a wait queue

#### Scenario: Kill switch restores legacy selection

- **GIVEN** a group is in `enforce`
- **WHEN** an operator changes its mode to `off`
- **THEN** subsequent selections use the legacy selector without restart
- **AND** scorer runtime statistics do not alter the legacy result

### Requirement: Legacy Schedulers Are Removed Only After Full Gates

The Gateway and OpenAI legacy scheduler bodies MUST remain available for shadow comparison and rollback until all target platforms pass local replay, staging HTTP/SSE/WS, representative normal and peak windows, scoped canary, fault injection, billing invariance, and paired rollback gates.

After those gates are approved, production selection SHALL use only the shared engine, policies, scorer, and platform adapters, and the duplicate legacy bodies SHALL be removed.

#### Scenario: One platform has not passed canary

- **GIVEN** any target platform has not completed its approved canary and rollback gates
- **WHEN** cleanup is considered
- **THEN** the legacy scheduler bodies are not deleted
- **AND** the architecture is not reported as fully unified

#### Scenario: All platform gates pass

- **GIVEN** every target platform has passed the required differential, canary, fault, billing, and rollback gates
- **WHEN** the cleanup task is approved and executed
- **THEN** duplicate Gateway/OpenAI scheduler bodies and transitional adapters are removed
- **AND** long-term replay fixtures continue to guard selection behavior
