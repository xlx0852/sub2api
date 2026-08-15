## ADDED Requirements

### Requirement: Retired provider account types are fail-closed

The system SHALL reject creation, conversion, activation, restoration, credential testing, and bulk reactivation of an account whose provider account type has been retired. Rejection MUST be enforced in the service layer even when an HTTP binding or frontend registry also removes the type.

#### Scenario: Administrator attempts to create a Bedrock account

- **WHEN** an administrator submits an account with `type=bedrock`
- **THEN** the request is rejected and no account, group binding, credential side effect, or scheduler snapshot is created

#### Scenario: Administrator attempts to reactivate a historical Bedrock account

- **WHEN** a single-account, bulk, restore, clear-error, or status-update operation would make a historical Bedrock account active or schedulable
- **THEN** the operation is rejected and the account remains inactive

### Requirement: Provider retirement preserves historical financial data

The retirement migration SHALL retain provider account rows and all historical usage, settlement, billing, and audit references while making every retired provider account inactive. The migration MUST be idempotent and MUST NOT modify accounts belonging to supported provider types.

#### Scenario: Retirement migration runs with existing Bedrock accounts

- **WHEN** migration 198 is applied to a database containing active, error, inactive, or soft-deleted Bedrock accounts
- **THEN** every Bedrock account is inactive, non-Bedrock accounts are unchanged, and historical references remain present

#### Scenario: Retirement migration is applied again

- **WHEN** migration 198 executes on an already migrated database
- **THEN** it completes without error or additional data changes

### Requirement: Retired provider supply has no production execution path

The system MUST NOT enumerate, select, probe, sign, forward, stream, settle through a provider-specific adapter, or expose account creation UI for AWS Bedrock after retirement. Shared API-key behavior for supported providers SHALL remain unchanged.

#### Scenario: Normal traffic is scheduled after Bedrock retirement

- **WHEN** a request is admitted for any supported platform
- **THEN** no Bedrock candidate or Bedrock transport path is reachable and the existing supported-platform protocol and billing behavior is preserved

#### Scenario: Administrator opens account management

- **WHEN** the create, edit, bulk-edit, filter, or model-whitelist UI is rendered
- **THEN** Bedrock is not offered as an account supply type or credential form

### Requirement: Historical policy scopes remain readable

The system SHALL continue to parse, validate, serialize, display, and save the historical `bedrock` beta-policy scope value, but that scope MUST NOT match any runtime account after Bedrock supply retirement.

#### Scenario: Existing settings contain a Bedrock-scoped rule

- **WHEN** settings with `scope=bedrock` are loaded and saved without changing that rule
- **THEN** the value round-trips unchanged and no validation error or silent rewrite occurs

#### Scenario: A Bedrock-scoped rule is evaluated

- **WHEN** policy evaluation runs for any supported runtime account
- **THEN** the Bedrock-scoped rule does not match
