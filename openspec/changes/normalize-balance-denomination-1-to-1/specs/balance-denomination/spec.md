## ADDED Requirements

### Requirement: Purchasing Power Preservation

The system SHALL preserve each user's purchasing power when converting the balance denomination from effective `1:10` to `1:1`.

#### Scenario: Existing balance and request charge are converted

- **WHEN** an old balance and its corresponding customer charge are migrated
- **THEN** both values SHALL be divided by `10`
- **AND** the number of equivalent requests purchasable with that balance SHALL remain unchanged

### Requirement: One-to-One Recharge

After cutover, the system SHALL credit one balance unit for each one unit of recharge principal.

#### Scenario: New balance recharge

- **WHEN** a user creates and completes a balance recharge for principal `P`
- **THEN** the credited balance SHALL equal `P`
- **AND** payment fees SHALL remain separate from the credited principal

### Requirement: Atomic Denomination Migration

The migration SHALL update all customer-denominated balances, limits, rates and histories atomically while application writes are stopped.

#### Scenario: Migration succeeds

- **WHEN** every conversion statement and reconciliation guard succeeds
- **THEN** the transaction SHALL commit once
- **AND** no row SHALL remain in the old customer denomination

#### Scenario: Migration validation fails

- **WHEN** any conversion statement or reconciliation guard fails
- **THEN** the transaction SHALL roll back completely
- **AND** production traffic SHALL remain disabled until the old state is restored

### Requirement: Raw Cost Preservation

The migration SHALL preserve actual payment amounts, raw upstream model costs and account-side cost accounting.

#### Scenario: Raw cost history is inspected after migration

- **WHEN** an operator compares a usage row before and after migration
- **THEN** raw token costs and account statistics cost SHALL be unchanged
- **AND** only the customer charge and customer rate multiplier SHALL use the new denomination

### Requirement: Consistent Customer Display

All customer-visible monetary balance, quota, usage and affiliate values SHALL use the new denomination after cutover.

#### Scenario: User reviews account history

- **WHEN** a user opens balance, API-key quota, usage or affiliate pages after cutover
- **THEN** all displayed customer-denominated amounts SHALL be mutually consistent
- **AND** the system SHALL explain that balances and charges were scaled together without reducing available usage

### Requirement: Recoverable Cutover

The operation SHALL retain a verified pre-migration backup and previous application bundle until post-cutover monitoring completes.

#### Scenario: Validation fails before reopening traffic

- **WHEN** post-migration reconciliation or canary billing fails
- **THEN** operators SHALL restore the complete pre-migration database and previous application/config bundle
- **AND** traffic SHALL not be reopened on a mixed-denomination state
