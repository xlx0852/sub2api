## ADDED Requirements

### Requirement: Tiered enforcement for moderation hits

The system SHALL support an optional first-level enforcement action that disables only the API Key responsible for a countable content-moderation violation, while retaining user-level disablement as the higher-level action after the configured violation threshold is reached.

#### Scenario: A first countable hit disables only the current API Key

- **WHEN** the API-Key disable strategy is enabled and an active API Key produces a countable moderation hit below the user ban threshold
- **THEN** the system disables that API Key, invalidates its authentication cache, keeps the user account and the user's other API Keys unchanged, and records the applied action

#### Scenario: Repeated hits escalate to user disablement

- **WHEN** a user's countable moderation hits reach the configured threshold within the configured window
- **THEN** the system disables the user account according to the existing auto-ban policy in addition to any first-level API-Key action

#### Scenario: The responsible API Key cannot be safely identified

- **WHEN** a countable hit has no API Key identifier, the Key does not belong to the user, or the Key is already disabled
- **THEN** the system does not disable a different API Key and records the non-applied first-level action

#### Scenario: First-level enforcement is not enabled

- **WHEN** the API-Key disable strategy is absent or disabled
- **THEN** the system preserves the existing request-blocking and user auto-ban behavior without changing API Key status

### Requirement: Tiered enforcement auditability

The system SHALL expose and notify the actual enforcement result separately for API-Key disablement and user-account disablement.

#### Scenario: State mutation fails after audit preparation

- **WHEN** an API-Key or user status update fails
- **THEN** the system records the failure, does not claim that the action succeeded, and leaves unrelated credentials unchanged

### Requirement: Cyber policy exclusion remains authoritative

The system SHALL preserve the existing configuration that excludes `cyber_policy` events from automatic ban counting and enforcement.

#### Scenario: Cyber policy events are excluded

- **WHEN** `cyber_policy` exclusion is enabled and a `cyber_policy` event is recorded
- **THEN** the event remains visible in risk-control logs and notifications but neither disables the API Key nor contributes to user-account disablement
