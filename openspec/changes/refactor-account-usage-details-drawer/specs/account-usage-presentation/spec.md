## ADDED Requirements

### Requirement: Compact account usage summary

The account management table SHALL present a compact and stable usage summary for each account without rendering the complete platform-specific usage payload inline.

#### Scenario: Account has official quota windows
- **GIVEN** an account has one or more official upstream quota windows
- **WHEN** an administrator views the account list
- **THEN** the summary SHALL show at most two primary quota windows with utilization and reset information
- **AND** the summary SHALL show the account's current risk state and today's local consumption

#### Scenario: Account has no official quota
- **GIVEN** an account has local usage statistics but no official upstream quota
- **WHEN** an administrator views the account list
- **THEN** the summary SHALL show local request, token, and cost information
- **AND** the interface SHALL NOT present local usage as an official quota percentage

### Requirement: Account usage details drawer

The account management interface SHALL provide a right-side details drawer for the selected account.

#### Scenario: Administrator opens account usage details
- **WHEN** an administrator activates an account usage summary
- **THEN** the interface SHALL open a details drawer without leaving the account list
- **AND** the drawer SHALL identify the selected account, platform, plan, and current status

#### Scenario: Administrator switches accounts
- **GIVEN** the details drawer is open
- **WHEN** the administrator selects another account in the list
- **THEN** the drawer SHALL update to the newly selected account without requiring the drawer to be closed

### Requirement: Semantic usage detail categories

The details drawer SHALL separate current quota, historical statistics, performance, and diagnostics into distinct views within the same drawer.

#### Scenario: Administrator reviews quota
- **WHEN** the administrator selects the quota view
- **THEN** the drawer SHALL show upstream quota utilization, reset times, plan information, source, and freshness

#### Scenario: Administrator reviews historical consumption
- **WHEN** the administrator activates the statistics action from the drawer
- **THEN** the interface SHALL open the statistics tab in the same drawer for local requests, tokens, account cost, user charges, trends, and model breakdowns
- **AND** the drawer SHALL NOT duplicate those historical aggregates

#### Scenario: Administrator reviews performance
- **WHEN** the administrator selects the performance view
- **THEN** the drawer SHALL show available latency, success, and connection metrics for the account

#### Scenario: Administrator reviews diagnostics
- **WHEN** the administrator selects the diagnostics view
- **THEN** the drawer SHALL show data source, last update, response state, and actionable authorization or probe errors

### Requirement: Consistent usage actions

Equivalent usage operations SHALL use consistent labels and behavior across supported platforms.

#### Scenario: Administrator refreshes upstream quota
- **WHEN** an administrator activates `刷新额度`
- **THEN** the system SHALL perform the platform's active upstream quota request
- **AND** the action SHALL NOT be labeled as local statistics refresh

#### Scenario: Administrator opens local statistics
- **WHEN** an administrator activates `查看统计`
- **THEN** the system SHALL open the local statistics tab in the details drawer without implying that upstream quota was queried

### Requirement: Responsive and accessible details

The usage summary and details interface SHALL remain usable on desktop and narrow viewports.

#### Scenario: Narrow viewport
- **WHEN** the account usage details are opened on a narrow viewport
- **THEN** the details surface SHALL use the available width without overlapping the account table or clipping text

#### Scenario: Keyboard operation
- **WHEN** an administrator navigates the usage interface by keyboard
- **THEN** summaries, tabs, actions, and drawer dismissal SHALL be reachable and expose visible focus states
