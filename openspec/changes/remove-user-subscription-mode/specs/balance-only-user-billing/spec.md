## ADDED Requirements

### Requirement: User traffic uses balance-only billing

The system SHALL authorize and charge all user API traffic through the user's balance. It SHALL NOT grant request access from a `UserSubscription`, skip balance checks because of a subscription group, or increment user subscription quota windows.

#### Scenario: Former subscription user sends a request

- **WHEN** a user who previously had an active user subscription sends a request after the migration
- **THEN** the system SHALL apply the same balance eligibility and debit rules as every other user
- **AND THEN** it SHALL not load or attach a user subscription to the gateway request context

#### Scenario: New usage is recorded

- **WHEN** a post-migration request is successfully billed
- **THEN** the usage record SHALL have no user subscription association
- **AND THEN** it SHALL use the balance billing type

### Requirement: User subscription management is unavailable

The system SHALL NOT expose user subscription assignment, extension, revocation, restoration, quota reset, listing, purchase, redemption, or automatic assignment capabilities through production routes or background jobs.

#### Scenario: Old client calls a subscription endpoint

- **WHEN** a client calls a removed admin or user subscription route
- **THEN** the system SHALL return route-not-found behavior
- **AND THEN** it SHALL not create or mutate a user subscription

#### Scenario: Subscription creation source is triggered

- **WHEN** a payment, redeem code, OAuth first bind, or default assignment policy previously configured for user subscriptions is processed
- **THEN** the system SHALL not create or extend a user subscription
- **AND THEN** non-subscription fulfillment SHALL continue according to its existing contract

### Requirement: Groups support standard billing only

The system SHALL treat all groups as standard balance-billed groups and SHALL reject attempts to create or update a group with user subscription mode. Administrative interfaces SHALL not display user subscription type or subscription quota configuration.

#### Scenario: Existing subscription group is migrated

- **WHEN** the removal migration processes a group whose `subscription_type` is `subscription`
- **THEN** it SHALL convert the group to standard billing
- **AND THEN** requests routed through that group SHALL require and debit user balance

#### Scenario: Administrator edits a group

- **WHEN** an administrator creates or edits a group after the migration
- **THEN** the interface SHALL not offer subscription mode, validity days, or subscription daily/weekly/monthly quota fields
- **AND THEN** the backend SHALL reject a crafted subscription-mode request

### Requirement: Historical subscription records remain auditable

The system SHALL preserve historical user subscriptions, subscription-linked usage logs, payment orders, and redemption records for read-only audit purposes while ensuring they cannot authorize new traffic or create new subscription state.

#### Scenario: Auditor reviews historical usage

- **WHEN** historical usage contains a non-null `subscription_id`
- **THEN** the record SHALL remain queryable with its original attribution
- **AND THEN** the historical association SHALL not reactivate or authorize a user subscription

#### Scenario: Active subscription state is retired

- **WHEN** the migration encounters an active or suspended user subscription
- **THEN** it SHALL transition the record to a non-authorizing retired state
- **AND THEN** it SHALL preserve the original dates, usage counters, assignee, notes, and identifiers

### Requirement: Account procurement subscription accounting remains intact

The system SHALL preserve account-level subscription procurement accounting used by profit analysis, including OAuth / Setup Token cost classification and `account_subscription_cycles`.

#### Scenario: Profit report includes an OAuth account

- **WHEN** profit analysis calculates an OAuth or Setup Token account after user subscription mode is removed
- **THEN** it SHALL continue using the account's confirmed procurement cycle ledger
- **AND THEN** removing `UserSubscription` SHALL not alter account procurement cost or profit
