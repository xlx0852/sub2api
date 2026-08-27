## ADDED Requirements

### Requirement: Gateway First-Output Latency Is Measured In Reconciled Stages

The system SHALL record gateway-side end-to-end first-output latency and mutually exclusive stage durations for successful streaming requests, including gateway preparation, upstream connection acquisition, upstream first-output wait, retry/failover, and gateway first-output flush. The existing `first_token_ms` field SHALL retain its current semantics for compatibility.

#### Scenario: Streaming request succeeds without retry

- **GIVEN** a streaming request produces visible output on its first upstream attempt
- **WHEN** the gateway flushes the first visible output to its downstream writer
- **THEN** the usage record contains a gateway first-output total and all supported stage durations
- **AND** the supported stage durations reconcile with the total within the documented tolerance
- **AND** connection reuse is represented as measured acquisition time rather than a fabricated delay

#### Scenario: Streaming request succeeds after account failover

- **GIVEN** one or more upstream attempts fail before visible output
- **WHEN** an untried account produces the first visible output
- **THEN** the gateway first-output total starts at the original request admission time
- **AND** failed-attempt, backoff, and account-switch time is included in the retry/failover stage
- **AND** the final successful attempt does not reset the gateway first-output total

#### Scenario: A stage is unsupported or unavailable

- **GIVEN** a transport cannot provide a reliable timestamp for a stage
- **WHEN** the usage record is persisted
- **THEN** that stage remains null
- **AND** the system does not store or display the missing stage as zero latency

### Requirement: Ops Dashboard Shows Average First-Output Stage Latency

The Ops Dashboard SHALL show the selected filter window's average latency for each available first-output stage and the gateway first-output total. Each average SHALL include a sample count and coverage ratio, and long-window aggregation SHALL remain exact for averages by combining sums and sample counts.

#### Scenario: Admin inspects a recent raw window

- **GIVEN** successful streaming usage rows contain first-output stage measurements
- **WHEN** an admin opens the Ops Dashboard for a recent window
- **THEN** the dashboard shows average milliseconds for gateway preparation, connection acquisition, upstream first-output wait, retry/failover, first-output flush, and the gateway total
- **AND** the dashboard shows how much of the eligible streaming traffic contributed to each average

#### Scenario: Admin inspects a mixed raw and hourly window

- **GIVEN** the selected range contains pre-aggregated full hours and raw head or tail fragments
- **WHEN** the dashboard computes stage averages
- **THEN** each result equals the combined stage sum divided by the combined stage sample count
- **AND** missing historical measurements do not dilute the average with zeros

### Requirement: First-Output Labels State The Measurement Boundary

The dashboard SHALL describe the stage breakdown as gateway-side measurement and SHALL NOT label `upstream_first_output_ms` as pure OpenAI inference time.

#### Scenario: Admin reads the stage tooltip

- **WHEN** an admin views the first-output stage help text
- **THEN** it states that client-to-gateway upload, gateway-to-client Internet delivery, and client rendering are not measured
- **AND** it states that upstream first-output wait includes provider processing plus upstream network time
