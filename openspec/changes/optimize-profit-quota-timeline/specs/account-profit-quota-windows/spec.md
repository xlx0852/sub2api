## ADDED Requirements

### Requirement: Quota timeline rendering remains bounded

The Profit quota timeline SHALL keep its rendered grid nodes, projection work, and horizontal content width bounded independently of the number of account lanes and the user's accumulated horizontal navigation distance.

#### Scenario: Many accounts share one visible timeline grid

- **WHEN** the Profit view renders the maximum supported account lanes
- **THEN** visible time-grid nodes SHALL be rendered once for the timeline rather than once per account lane
- **AND THEN** each lane SHALL retain its own selectable quota-window bars

#### Scenario: User continuously scrolls through time

- **WHEN** the user repeatedly reaches either horizontal edge
- **THEN** the timeline SHALL shift its absolute-time baseline while preserving a fixed content span
- **AND THEN** the visible absolute time SHALL remain continuous without permanently growing the content width

### Requirement: Timeline optimization preserves quota semantics

The optimized timeline SHALL preserve real ledger windows, fallback projections, subscription availability clipping, waiting-activation gaps, drifting windows, and the window-selection payload contract.

#### Scenario: Optimized timeline contains mixed window states

- **WHEN** an account has ended, waiting-activation, active, and projected intervals within the visible range
- **THEN** each interval SHALL retain its existing status and subscription-bound clipping
- **AND THEN** selecting a bar SHALL identify the same account and source interval as before the optimization

