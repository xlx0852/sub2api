## ADDED Requirements

### Requirement: Paid subscription cycles bound quota availability

The system SHALL treat each recorded paid subscription cycle as the authoritative availability and cost boundary. Provider quota windows SHALL be intersected with recorded subscription-cycle spans, and no provider window or projection SHALL expose usable supply after the controlling paid cycle ends.

#### Scenario: Provider window crosses subscription expiry

- **WHEN** a provider window ends after the recorded subscription cycle ends
- **THEN** profit analysis SHALL expose only the intersection ending at subscription expiry
- **AND THEN** time after expiry SHALL NOT be reported as usable quota supply

### Requirement: Provider clearing and window activation are separate events

The system SHALL close the prior provider window when its quota is cleared. If the next rolling window starts only on first use, the system SHALL NOT open a replacement window until a real active-window observation is received.

#### Scenario: Weekly quota clears while the account is idle

- **WHEN** the provider clears weekly usage at 01:00 and the first real model use occurs at 09:00
- **THEN** the prior window SHALL end at 01:00
- **AND THEN** the interval from 01:00 to 09:00 SHALL be represented as waiting for activation
- **AND THEN** the next provider window SHALL start at 09:00

### Requirement: Early provider resets preserve real boundaries

The system SHALL recognize a credible forward jump in the observed provider reset time as a new window even when the new theoretical period overlaps the prior window's former expected interval.

#### Scenario: Provider resets a seven-day limit early

- **WHEN** an existing window was expected to end on August 8 and a provider reset on August 5 produces a new reset time on August 12
- **THEN** the old window SHALL close at the inferred August 5 activation boundary
- **AND THEN** a new window SHALL open from August 5 to August 12
- **AND THEN** the two observations SHALL NOT be merged into one August 1 to August 12 window

### Requirement: Waiting time remains subscription idle time

Waiting-for-activation time SHALL belong to the paid subscription cycle but SHALL NOT belong to either adjacent provider quota window. Fixed subscription cost SHALL remain unchanged.

#### Scenario: Profit analysis includes an activation gap

- **WHEN** a paid account has an eight-hour gap between provider clearing and first-use activation
- **THEN** window-level analysis SHALL not assign requests or revenue from that gap to either provider window
- **AND THEN** subscription-cycle profit SHALL continue to use the full recorded purchase cost

### Requirement: Background observation does not alter lazy activation

Periodic quota observation SHALL use a read-only usage endpoint. A model inference probe that can start a lazy rolling window SHALL NOT be used to maintain the quota ledger for an account waiting for activation.

#### Scenario: Account remains unused overnight

- **WHEN** the provider cleared quota and no real user request has occurred
- **THEN** background ledger maintenance SHALL NOT start the next rolling window

