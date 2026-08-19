## MODIFIED Requirements

### Requirement: Metered profit uses a manually controlled comprehensive cost multiplier

The system SHALL calculate API Key account cost from the account-side numeric model cost multiplied by the historical comprehensive cost multiplier stored with each usage record. The comprehensive multiplier MAY include upstream discount, operator-controlled exchange-rate treatment, fees, and risk buffer. The system SHALL calculate revenue from the user's actual charged credits independently and SHALL NOT automatically convert either value using a foreign-exchange service.

#### Scenario: Comprehensive cost multiplier includes operator-controlled FX

- **WHEN** an API Key account has a historical comprehensive cost multiplier of `0.26` and a request has an account-side numeric base cost of `3.00`
- **THEN** the system SHALL record numeric cost `0.78` for profit calculation
- **AND THEN** it SHALL NOT apply an additional currency conversion

#### Scenario: Customer selling multiplier differs from comprehensive cost multiplier

- **WHEN** a request is sold at a group multiplier of `0.30` while the account comprehensive cost multiplier is `0.26`
- **THEN** the system SHALL retain the customer charge and account cost as separate credit values
- **AND THEN** the profit report SHALL calculate profit from the numeric customer charge minus the numeric account cost
- **AND THEN** it SHALL not treat either multiplier as an automatic exchange rate

#### Scenario: Historical purchase discount snapshot remains readable

- **WHEN** a usage record predates the comprehensive-cost terminology
- **THEN** its stored account multiplier SHALL continue to participate in the same numeric cost formula
- **AND THEN** the system SHALL NOT rewrite the historical multiplier or account cost
