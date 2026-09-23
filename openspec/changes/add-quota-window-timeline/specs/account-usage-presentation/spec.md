## ADDED Requirements

### Requirement: Quota windows timeline

The account management list SHALL provide a timeline that draws each listed account's quota windows on a
shared time axis, so an administrator can see when capacity returns and whether resets are staggered or
simultaneous, without replacing the existing per-account summary.

#### Scenario: Windows are projected from the reported reset instant

- **GIVEN** an account reports a quota window with a reset instant and a utilization
- **WHEN** the administrator views the timeline
- **THEN** the account SHALL occupy one lane
- **AND** the lane SHALL draw one bar per window occurrence, placed by shifting the reset instant by
  whole window periods across the visible span

#### Scenario: Current, upcoming, and elapsed windows are distinguishable

- **WHEN** a lane is drawn
- **THEN** the window ending at the reported reset instant SHALL be marked as current and carry the
  reported utilization
- **AND** windows after it SHALL be marked as upcoming and SHALL NOT show a utilization
- **AND** windows before it SHALL be marked as elapsed
- **AND** the current moment SHALL be marked on the axis when it falls inside the visible span

#### Scenario: The span is zoomable between a weekly and a 5-hour view

- **WHEN** the administrator switches the zoom
- **THEN** the weekly view SHALL span a fortnight with one axis cell per day and project each account's
  longest window that fits the span
- **AND** the 5-hour view SHALL span three days with one axis cell per six hours and project only
  accounts that report a 5-hour window
- **AND** the administrator SHALL be able to step the span backwards and forwards and return to the
  current period

#### Scenario: An account without a usable reset instant stays visible

- **GIVEN** a listed account reports no usable reset instant for any window
- **WHEN** the administrator views the timeline
- **THEN** the account SHALL still occupy a lane
- **AND** the lane SHALL state that no window is counting down instead of drawing an invented bar

#### Scenario: Manual reset credits are marked

- **GIVEN** an account payload exposes an unexpired manual reset credit
- **WHEN** its expiry falls inside the visible span
- **THEN** the lane SHALL mark the expiry instant
- **AND** the marker SHALL expose the credit's granted and expiry instants on hover or focus

#### Scenario: The timeline agrees with the row summary

- **GIVEN** an account shows utilization and a reset countdown in its row summary
- **WHEN** the same account is drawn in the timeline
- **THEN** the current window's bar SHALL carry the same utilization and end instant
- **AND** the timeline SHALL read that account's usage through the same cached loader as the row summary
  rather than issuing a parallel request

#### Scenario: The timeline stays readable on a narrow viewport

- **WHEN** the viewport is too narrow for the label column plus the axis
- **THEN** the chart SHALL scroll horizontally with a minimum width rather than compressing the axis
  until labels are illegible
