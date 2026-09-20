## ADDED Requirements

### Requirement: Admin balance increases accrue affiliate rebates

The system SHALL treat a positive balance delta produced by an admin balance adjustment as a recharge for affiliate rebate calculation, using the existing affiliate policy settings and inviter relationship.

#### Scenario: Admin adds balance to an invitee

- **WHEN** an administrator increases a user balance and the user has an eligible inviter
- **THEN** the system credits the inviter's affiliate quota according to the configured rebate policy
- **AND** the user balance adjustment remains recorded as an admin balance history entry

#### Scenario: Admin reduces or leaves balance unchanged

- **WHEN** an administrator subtracts balance or sets a balance to a lower/equal value
- **THEN** the system SHALL NOT accrue an affiliate rebate
