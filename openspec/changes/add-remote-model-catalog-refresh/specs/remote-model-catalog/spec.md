## ADDED Requirements

### Requirement: Layered catalog startup
The system SHALL synchronously load a valid last-known-good local model catalog when available, otherwise load the embedded catalog, and SHALL NOT require remote network access to start serving requests.

#### Scenario: Remote source is unavailable during startup
- **WHEN** the service starts while the configured remote source cannot be reached
- **THEN** the service serves requests using the valid local or embedded catalog
- **AND** records the remote failure without clearing the active catalog

### Requirement: Authenticated remote catalog candidate
The system SHALL treat every remote catalog as an untrusted candidate and SHALL enforce configured URL policy, request timeout, response size, SHA-256, JSON, version, and business validation before activation.

#### Scenario: Remote checksum does not match
- **WHEN** the downloaded catalog SHA-256 differs from the published checksum
- **THEN** the system rejects the candidate
- **AND** retains the active catalog and last-known-good file

#### Scenario: Remote catalog violates model constraints
- **WHEN** a candidate contains duplicate model IDs, invalid aliases, negative prices, or cyclic pricing aliases
- **THEN** the system rejects the complete candidate without partially applying it

### Requirement: Atomic catalog activation
The system SHALL build a complete immutable catalog snapshot before atomically replacing the active snapshot.

#### Scenario: Requests overlap a successful refresh
- **WHEN** model catalog readers execute while a valid candidate is activated
- **THEN** every reader observes either the complete previous snapshot or the complete new snapshot
- **AND** no reader observes partially updated platforms, mappings, or pricing

### Requirement: Last-known-good persistence
The system SHALL persist each accepted remote catalog as a last-known-good local snapshot using an atomic file replacement strategy.

#### Scenario: Process restarts after a successful remote refresh
- **WHEN** the remote source is unavailable after restart
- **THEN** the service loads the previously accepted local snapshot before falling back to the embedded catalog

### Requirement: Background refresh lifecycle
The system SHALL check for remote catalog changes asynchronously after startup and periodically thereafter, with cancellation, bounded concurrency, and randomized scheduling jitter.

#### Scenario: Scheduled and manual refresh overlap
- **WHEN** a manual refresh is requested while a scheduled refresh is running
- **THEN** the system coalesces or rejects duplicate work deterministically
- **AND** does not perform concurrent activation of multiple candidates

### Requirement: Semantic change propagation
The system SHALL classify catalog changes by affected platform and semantic category and SHALL invalidate only derived runtime state that depends on the changed data.

#### Scenario: Display metadata changes only
- **WHEN** a refresh changes only display names, media metadata, or UI presets
- **THEN** catalog and UI readers observe the new metadata
- **AND** account scheduling state is not rebuilt

#### Scenario: Model mapping changes
- **WHEN** a refresh changes model IDs, retired IDs, aliases, or default mappings for one platform
- **THEN** the system invalidates affected model availability and mapping-derived state for that platform
- **AND** leaves unrelated platform state intact

### Requirement: Runtime consumers follow active snapshot
The system SHALL resolve model lists, default models, aliases, mappings, fallback prices, and UI presets from the active catalog snapshot rather than package-initialization copies.

#### Scenario: Default model changes at runtime
- **WHEN** a valid activated catalog changes a platform default model
- **THEN** requests created after activation use the new default without restarting the process

### Requirement: Operational policy isolation
The system SHALL NOT automatically convert remote catalog entries into account capability, account whitelist, channel pricing, tenant routing, or custom group model-list changes.

#### Scenario: Catalog adds a model unsupported by all accounts
- **WHEN** the remote catalog introduces a new model ID but no configured account exposes it
- **THEN** the model is available as catalog metadata only
- **AND** is not treated as schedulable solely because it exists in the catalog

### Requirement: Administrative status and refresh
The system SHALL expose authenticated administrator operations to inspect catalog refresh status and request an immediate refresh without exposing remote response bodies or secrets.

#### Scenario: Administrator inspects status after failure
- **WHEN** the latest refresh failed
- **THEN** the status contains the active version, source, hash, last check, last success, and a sanitized error summary
- **AND** confirms that the previous catalog remains active

### Requirement: Configurable and disableable remote source
The system SHALL allow operators to replace the default remote URLs and disable remote refresh while retaining local and embedded loading.

#### Scenario: Operator disables remote refresh
- **WHEN** `model_catalog.remote_enabled` is false
- **THEN** the service performs no remote catalog requests
- **AND** continues using the valid local or embedded catalog
