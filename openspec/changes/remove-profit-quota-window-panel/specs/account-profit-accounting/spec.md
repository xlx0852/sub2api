## MODIFIED Requirements

### Requirement: Quota windows stop at subscription availability end

The system SHALL stop projecting quota-window occurrences for a subscription account at the controlling subscription cycle end, or earlier at the confirmed ban effective timestamp. Non-subscription accounts SHALL retain their existing rolling-window behavior.

#### Scenario: Subscription cycle expires

- **WHEN** a subscription account's current or latest controlling cycle reaches its end
- **THEN** profit analysis SHALL NOT report a quota window whose start is at or after that end

#### Scenario: Account is confirmed banned during a cycle

- **WHEN** a subscription cycle has an active termination with an effective timestamp before the cycle end
- **THEN** profit analysis SHALL stop window projection at the ban effective timestamp and SHALL NOT report a subsequent window

#### Scenario: Metered account has a rolling snapshot

- **WHEN** a non-subscription account has a quota snapshot
- **THEN** the existing rolling-window projection SHALL remain unchanged
