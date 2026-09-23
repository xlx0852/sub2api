## ADDED Requirements

### Requirement: Usage records capture the upstream response model

Every usage record SHALL persist the model name declared by the upstream response, before any protocol
conversion or client-facing rewrite, and SHALL persist whether that model disagrees with the model the
gateway sent upstream. When the upstream response declares no model, both fields SHALL remain unset
rather than recording a guess.

#### Scenario: Upstream declares a different model

- **WHEN** the gateway sends `gpt-5.6-sol` upstream and the response declares `gpt-6-sol`
- **THEN** the usage record stores `gpt-6-sol` as the upstream response model and flags the record as mismatched

#### Scenario: Upstream confirms the sent model

- **WHEN** the upstream response declares the same model the gateway sent, ignoring letter case
- **THEN** the usage record stores the declared model and flags the record as not mismatched

#### Scenario: Mapped models compare against the sent model

- **WHEN** an account model mapping rewrote the request before forwarding
- **THEN** the comparison uses the mapped model that was actually sent, falling back to the requested model only when no mapping applied

#### Scenario: No declaration stays unset

- **WHEN** the upstream response declares no model
- **THEN** the usage record leaves both the upstream response model and the mismatch flag unset

### Requirement: The admin usage view surfaces upstream routing

The admin usage view SHALL display the upstream response model together with a mismatch indicator
whenever the upstream disagreed with the model sent, on both the desktop table and the mobile card, and
the CSV export SHALL include the upstream response model and the mismatch verdict.

#### Scenario: Mismatched row is visible

- **WHEN** a usage row carries a mismatched upstream response model
- **THEN** the model column shows the upstream response model and a mismatch badge

#### Scenario: Matching or unrecorded rows stay uncluttered

- **WHEN** a usage row's upstream response model matches the model sent, or was never recorded
- **THEN** the model column shows no upstream response annotation

#### Scenario: Export carries the audit fields

- **WHEN** an operator exports usage records
- **THEN** each exported row includes the upstream response model and whether it mismatched

### Requirement: The audit fields are additive and back-compatible

Adding the audit fields SHALL NOT change existing pricing, routing, or billing behaviour, and rows
written before the columns existed SHALL read back with both fields unset.

#### Scenario: Historical rows remain readable

- **WHEN** a usage row created before the audit columns existed is read
- **THEN** the row loads with both audit fields unset and all other fields unchanged
