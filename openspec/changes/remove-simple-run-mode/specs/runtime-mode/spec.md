## REMOVED Requirements

### Requirement: Simple runtime mode

The system SHALL no longer provide a simple runtime mode that bypasses standard billing, quota, grouping, scheduling, initialization, or navigation behavior.

#### Scenario: Legacy simple configuration is rejected

- **WHEN** an operator starts the service with `RUN_MODE=simple` or `run_mode: simple`
- **THEN** startup SHALL fail with an actionable message requiring migration to standard mode
- **AND** the system SHALL NOT silently start with standard billing and scheduling semantics

#### Scenario: Standard mode remains supported

- **WHEN** an operator omits the run mode configuration or explicitly selects `standard`
- **THEN** the service SHALL start with the existing standard authentication, billing, quota, grouping, and scheduling behavior

#### Scenario: Clients receive no runtime-mode branch signal

- **WHEN** a client authenticates or loads application state
- **THEN** API responses SHALL NOT expose a `run_mode` field for frontend behavior branching
- **AND** frontend navigation and features SHALL be governed only by roles, permissions, and feature flags

#### Scenario: Existing business data is preserved

- **WHEN** an installation previously used simple mode and upgrades after completing configuration migration
- **THEN** existing users, groups, API keys, accounts, and channels SHALL remain intact
- **AND** the upgrade SHALL NOT automatically delete records previously created by simple-mode initialization
