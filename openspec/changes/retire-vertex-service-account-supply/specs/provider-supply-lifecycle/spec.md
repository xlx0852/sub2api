## ADDED Requirements

### Requirement: Retired Vertex Service Account supply has no production execution path

The system SHALL reject creation, conversion, activation, restoration, credential testing, error clearing, and bulk reactivation of a Vertex Service Account. The system MUST NOT enumerate, select, probe, exchange tokens, transform, forward, stream, settle new traffic, or submit batch-image work through a Vertex Service Account after retirement. Enforcement MUST exist below the HTTP and frontend layers.

#### Scenario: Administrator attempts to create a Vertex Service Account

- **WHEN** an administrator submits a single or bulk account creation request with `type=service_account` for Anthropic or Gemini
- **THEN** the request is rejected and no account, credential, group binding, scheduler snapshot, or other creation side effect is persisted

#### Scenario: Administrator opens the account creation flow

- **WHEN** the Anthropic or Gemini account creation options are rendered
- **THEN** `Vertex / Service Account` is not offered and all supported OAuth and API-key options remain available

#### Scenario: Administrator attempts to reactivate a historical Vertex account

- **GIVEN** a Vertex Service Account existed before retirement
- **WHEN** a single, bulk, restore, clear-error, credential-test, or status operation would make it active, schedulable, or invoke its credentials
- **THEN** the operation is rejected and the account remains inactive and non-schedulable

#### Scenario: Traffic is scheduled after Vertex retirement

- **WHEN** ordinary Gemini, Anthropic, or batch-image traffic is admitted
- **THEN** no Vertex Service Account candidate, token exchange, protocol adapter, or batch Provider is reachable

### Requirement: Vertex retirement preserves historical records

The retirement migration SHALL retain historical Vertex account rows, encrypted credentials, group references, usage, billing, settlement, and audit records while making every `service_account` account inactive and non-schedulable. The migration MUST be idempotent and MUST NOT modify supported account types.

#### Scenario: Retirement migration runs with existing Vertex accounts

- **WHEN** the migration is applied to active, error, inactive, or soft-deleted Service Account rows
- **THEN** every matching live row is inactive and non-schedulable, non-Service-Account rows are unchanged, and historical references remain present

#### Scenario: Retirement migration is applied again

- **WHEN** the migration executes on an already migrated database
- **THEN** it completes without error or additional data changes

#### Scenario: Administrator views historical accounts

- **WHEN** account management or data export reads a retired Vertex row
- **THEN** the row remains identifiable and deletable but exposes no action that can test, restore, schedule, or reactivate it
