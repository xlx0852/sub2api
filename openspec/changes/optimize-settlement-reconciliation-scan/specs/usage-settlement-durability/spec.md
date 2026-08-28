## ADDED Requirements

### Requirement: Indexed overdue settlement lookup

The system SHALL provide an online index that serves overdue `started` settlement lookup by reconciliation deadline without scanning settled history.

#### Scenario: No settlement attempts are overdue

- **WHEN** the reconciliation worker checks a large settlement table with no overdue `started` rows
- **THEN** PostgreSQL uses the partial started-reconciliation index
- **AND** does not scan settled rows

### Requirement: Independent reconciliation cadence

The system SHALL schedule overdue settlement recovery independently from request-driven settlement draining.

#### Scenario: A completed request wakes the settlement worker

- **WHEN** an authoritative settlement handoff calls `Wake`
- **THEN** the worker drains ready settlement attempts promptly
- **AND** does not run overdue provider recovery or expiry scans because of that wakeup

#### Scenario: Reconciliation interval elapses

- **WHEN** the dedicated reconciliation interval elapses
- **THEN** the worker first attempts provider recovery for eligible overdue rows
- **AND** then marks remaining overdue rows as requiring reconciliation

#### Scenario: Worker starts in an enabled mode

- **WHEN** the settlement worker starts in shadow or authoritative mode
- **THEN** it runs one reconciliation pass without waiting for the first periodic interval
