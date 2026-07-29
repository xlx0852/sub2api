## ADDED Requirements

### Requirement: Automatic Grok Usage Synchronization

The system SHALL automatically synchronize official Grok billing usage for OAuth accounts when the persisted billing snapshot is missing or older than the configured freshness window.

#### Scenario: Missing Grok billing snapshot

- **WHEN** an administrator requests usage for a Grok OAuth account with no persisted billing snapshot
- **THEN** the system probes the official Grok billing endpoints
- **AND** returns usage built from the newly persisted snapshot

#### Scenario: Stale Grok billing snapshot

- **WHEN** an administrator requests usage for a Grok OAuth account whose billing snapshot is older than ten minutes
- **THEN** the system refreshes the snapshot before returning usage

#### Scenario: Fresh Grok billing snapshot

- **WHEN** an administrator requests usage for a Grok OAuth account whose billing snapshot is less than ten minutes old
- **THEN** the system returns the persisted snapshot without probing the upstream billing endpoints

### Requirement: Bounded Automatic Probe Concurrency

The system SHALL suppress duplicate in-process automatic Grok billing probes for the same account.

#### Scenario: Concurrent usage reads

- **WHEN** multiple usage requests for the same stale Grok account arrive concurrently
- **THEN** at most one upstream billing probe runs for that account
- **AND** all callers receive usage based on the resulting persisted snapshot

### Requirement: Automatic Sync Failure Degradation

The system SHALL keep Grok usage readable when an automatic upstream billing probe fails.

#### Scenario: Stale snapshot and upstream failure

- **WHEN** a stale persisted Grok snapshot exists and the automatic billing probe fails
- **THEN** the system returns usage based on the stale persisted snapshot
- **AND** does not convert the usage request into a hard failure

#### Scenario: No snapshot and upstream failure

- **WHEN** no Grok billing or rate-limit snapshot exists and the automatic billing probe fails
- **THEN** the system returns the existing quota-unknown degraded usage response

### Requirement: Manual Grok Usage Refresh

The system SHALL preserve an operator-triggered Grok quota refresh that bypasses automatic freshness suppression.

#### Scenario: Operator forces refresh

- **WHEN** an administrator activates Refresh quota for a Grok OAuth account
- **THEN** the system probes the official billing endpoints regardless of snapshot age
- **AND** persists and returns the refreshed billing snapshot
