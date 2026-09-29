# codex-client-version-sync Specification

## ADDED Requirements

### Requirement: Version Sync Runtime Participation
The system SHALL construct and start the Codex client version sync runner as part of application
assembly, and the shutdown assembly SHALL take that runner as a required dependency, so that an
injector graph that omits it fails to build instead of silently disabling the feature.

#### Scenario: Runner starts with the application
- **GIVEN** a server process starting with auto-sync enabled
- **WHEN** application assembly completes
- **THEN** the version sync runner is constructed and running with its configured interval
- **AND** it registers a stop function with the lifecycle registry

#### Scenario: Unreachable provider is a build failure
- **GIVEN** the version sync provider is present in the service provider set
- **WHEN** no injector node consumes it
- **THEN** the generated injector cannot satisfy the shutdown assembly's required parameter and the
  build fails
- **AND** the generated injector is consistent with the provider graph (`wire diff` reports nothing)

#### Scenario: Shutdown stops the runner
- **GIVEN** a running version sync runner
- **WHEN** the process shuts down through the lifecycle registry
- **THEN** the runner's stop function is invoked, its background goroutine exits, and no further
  version check is performed
- **AND** shutdown completes without panicking even when the runner was never started

### Requirement: Version Sync Observability
The runner SHALL log each synchronization decision with a stable event name and the reason for
skipping, so that a stale stored version can be distinguished from an upstream that published
nothing new.

#### Scenario: Start logs the effective interval
- **GIVEN** a runner created with a positive interval
- **WHEN** it starts
- **THEN** it logs a start event including the interval
- **AND** a runner with missing dependencies or a non-positive interval logs nothing and starts no
  goroutine

#### Scenario: Initial sync skipped for a fresh value
- **GIVEN** a synced value written less than one interval ago
- **WHEN** the runner starts
- **THEN** the startup check is skipped and logs the skip with the age of the stored value
- **AND** the value is left unchanged

#### Scenario: Skipped runs are logged with a reason
- **GIVEN** auto-sync disabled, or a stored value already equal to the latest stable release
- **WHEN** a synchronization run executes
- **THEN** it performs no write and logs a no-op event carrying the reason
- **AND** the reason distinguishes "disabled" from "already up to date" from "not advanced"

#### Scenario: Failures retain the stored value
- **GIVEN** a stored synced value and an upstream request that fails
- **WHEN** a synchronization run executes
- **THEN** the runner logs a failure event and keeps the stored value unchanged

### Requirement: Version Sync Semantics Preservation
The runner SHALL keep its existing synchronization semantics: forward-only advancement, default
enabled, candidate filtering, and no clearing of the stored value on error.

#### Scenario: Version never moves backwards
- **GIVEN** a stored synced value newer than the latest resolvable stable release
- **WHEN** a synchronization run executes
- **THEN** the stored value is not overwritten

#### Scenario: Only stable release tags are accepted
- **GIVEN** release entries mixing prereleases, non-`rust-v` tags, and stable `rust-v` tags
- **WHEN** the latest stable version is resolved
- **THEN** the highest stable `rust-v` version wins and the `rust-v` prefix is stripped from the
  stored value

#### Scenario: Sync is enabled by default
- **GIVEN** no stored value for the auto-sync toggle
- **WHEN** a synchronization run executes
- **THEN** the run proceeds as if enabled

### Requirement: Effective Client Version Resolution
The system SHALL resolve the effective Codex client version used for outbound identity and Codex
model manifests by preferring the administrator's panel value, then the synchronized value, and only
then the compiled fallback constant.

#### Scenario: Panel value wins
- **GIVEN** a panel value and a different synchronized value
- **WHEN** the effective client version is requested
- **THEN** the panel value is used and the synchronized value is not overwritten

#### Scenario: Synchronized value is used when the panel is empty
- **GIVEN** an empty panel value and a non-empty synchronized value
- **WHEN** the effective client version is requested
- **THEN** the synchronized value is used

#### Scenario: Fallback on unreadable or unusable settings
- **GIVEN** settings that cannot be read, or both values blank or malformed
- **WHEN** the effective client version is requested
- **THEN** the compiled fallback constant is used and the request succeeds without surfacing a
  settings error
