## ADDED Requirements

### Requirement: Application services are transport-neutral

Application services SHALL NOT import Gin or write HTTP/SSE/WebSocket frames directly. Streaming behavior SHALL be represented by transport-neutral events or a minimal sink interface, while protocol encoding and flushing SHALL remain in transport adapters.

#### Scenario: HTTP streaming request
- **WHEN** a gateway service produces a streaming response
- **THEN** it SHALL emit structured events through the application contract
- **AND** the handler/transport adapter SHALL encode SSE or JSON and control flushing

#### Scenario: WebSocket streaming request
- **WHEN** the same gateway capability is used over WebSocket
- **THEN** the service SHALL reuse the transport-neutral events
- **AND** the WebSocket adapter SHALL own frame encoding without importing Gin

### Requirement: Architecture verification configurations are buildable

The repository SHALL keep its declared unit-test and architecture-test configurations buildable. Every helper referenced by a tagged test MUST be available under the same build constraints, and CI SHALL execute the tagged unit/vet and architecture gates.

#### Scenario: Unit-tagged package test
- **WHEN** `go test -tags unit ./internal/service/...` is executed
- **THEN** the package SHALL compile without undefined helper symbols
- **AND** the test command SHALL report failures only from executed assertions

#### Scenario: New dependency violation
- **WHEN** a change introduces a service-to-transport or handler-to-persistence import
- **THEN** the architecture test SHALL fail before merge

### Requirement: Mutable runtime state is dependency-injected

Application runtime registries, provider clients, watchers, caches with mutable lifecycle, and other shared state SHALL be owned by an injected component or lifecycle registry. New package-level mutable variables and setter-based assembly bypasses MUST NOT be introduced.

#### Scenario: Provider client registry
- **WHEN** a provider needs account-scoped client reuse
- **THEN** the registry SHALL be supplied through construction
- **AND** tests SHALL be able to provide an isolated fake or empty registry without global reset

#### Scenario: Multiple application instances
- **WHEN** two application instances serve the same account
- **THEN** each instance SHALL have an explicit registry lifecycle
- **AND** no hidden package singleton SHALL be required for correctness

### Requirement: Persistence and cache contracts are centralized

Business services and transport handlers SHALL access persistence through ports/repositories rather than direct Ent clients or ad hoc SQL. Redis key construction and invalidation SHALL be centralized, and database mutations that affect cached scheduling state SHALL provide the corresponding invalidation as part of the repository operation.

#### Scenario: OAuth identity mutation
- **WHEN** an HTTP handler needs to query or update identity adoption data
- **THEN** it SHALL call an identity service/repository contract
- **AND** it SHALL NOT construct an Ent query directly

#### Scenario: Scheduling account mutation
- **WHEN** an account field affecting schedulability is updated
- **THEN** the repository operation SHALL invalidate or refresh the related scheduler snapshot
- **AND** callers SHALL NOT need to hand-write Redis key strings

### Requirement: Usage and settings schemas have one authoritative construction path

Usage-log insert columns SHALL be defined by one shared schema/builder used by single, batch, best-effort, and no-result writes. Settings SHALL be parsed through typed domain-specific schemas while preserving compatible legacy key/value storage.

#### Scenario: New usage field
- **WHEN** a usage-log field is added
- **THEN** all supported insert modes SHALL use the same ordered column definition
- **AND** a regression test SHALL verify column/value alignment for every mode

#### Scenario: Legacy setting key
- **WHEN** an existing setting key is loaded
- **THEN** the corresponding typed settings parser SHALL preserve its current meaning
- **AND** unknown keys SHALL remain safely ignorable during incremental migration

### Requirement: Capability packages have enforceable boundaries

Gateway, scheduling, billing/settlement, identity, and settings code SHALL be organized behind explicit package or interface boundaries. Shared scheduling primitives SHALL be single-sourced, while provider-specific policies remain explicit and behavior-preserving.

#### Scenario: Account selection behavior is shared
- **WHEN** Gateway and OpenAI select accounts with equivalent inputs
- **THEN** shared primitives SHALL produce equivalent candidate filtering and ordering
- **AND** provider-specific sticky or transport policies SHALL remain in their strategy layer

#### Scenario: New capability code is added
- **WHEN** a change adds code to one capability package
- **THEN** architecture checks SHALL reject imports that bypass the declared dependency direction
- **AND** the change SHALL not enlarge an unrelated god service or global registry
