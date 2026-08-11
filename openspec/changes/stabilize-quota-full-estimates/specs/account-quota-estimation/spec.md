## ADDED Requirements

### Requirement: Window-aligned quota observations

The system SHALL persist quota utilization observations aligned with local consumption for the same account quota window.

#### Scenario: Utilization changes in an open window
- **GIVEN** an account has an open quota window
- **WHEN** a fresh upstream snapshot reports a used percentage different from the latest observation
- **THEN** the system SHALL record the percentage and local cumulative consumption ending at the snapshot observation time
- **AND** the observation SHALL be associated with that exact quota window

#### Scenario: Snapshot repeats without utilization change
- **GIVEN** the latest observation already contains the reported used percentage for the same window
- **WHEN** another probe or traffic response reports the same percentage
- **THEN** the system SHALL NOT append a duplicate observation

#### Scenario: Observation persistence fails
- **WHEN** an observation cannot be persisted
- **THEN** request forwarding, upstream quota refresh, and account scheduling SHALL continue with their existing behavior
- **AND** the failure SHALL be available for diagnostics

### Requirement: Confidence-aware full-window estimate

The system SHALL estimate API list-price-equivalent full-window consumption from window-aligned observations without presenting the result as an upstream dollar quota.

#### Scenario: Utilization is below the minimum sample threshold
- **GIVEN** the current used percentage is below 5 percent
- **WHEN** account usage details are requested
- **THEN** the response SHALL mark the estimate as insufficient
- **AND** it SHALL omit point dollar estimates

#### Scenario: Valid incremental observations exist
- **GIVEN** at least two valid observations in the same window span at least 5 percentage points
- **WHEN** a full-window estimate is calculated
- **THEN** the estimate SHALL use consumption delta divided by utilization delta
- **AND** it SHALL return the method, confidence, sample count, percentage span, and estimate bounds

#### Scenario: Only a cumulative sample is available
- **GIVEN** current utilization is at least 5 percent
- **AND** no valid observation pair spans 5 percentage points
- **WHEN** a full-window estimate is calculated
- **THEN** the system MAY return the existing cumulative linear projection
- **AND** it SHALL label the result as low confidence

#### Scenario: Invalid samples are present
- **GIVEN** samples cross a reset boundary or contain decreasing time, cost, or utilization
- **WHEN** a full-window estimate is calculated
- **THEN** invalid samples SHALL be excluded from the estimate

### Requirement: Honest quota estimate presentation

The account usage interface SHALL communicate the estimate's local pricing basis and uncertainty.

#### Scenario: Estimate is available
- **WHEN** a low, medium, or high confidence estimate is returned
- **THEN** the interface SHALL label it as an API list-price-equivalent estimate
- **AND** it SHALL display confidence and the estimated range

#### Scenario: Estimate is insufficient
- **WHEN** the backend marks the estimate as insufficient
- **THEN** the interface SHALL explain that more quota utilization samples are required
- **AND** it SHALL NOT display a precise full-window dollar amount

