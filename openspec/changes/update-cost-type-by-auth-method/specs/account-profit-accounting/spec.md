## ADDED Requirements

### Requirement: Cost type derives from account authentication method

The system SHALL derive the profit cost type from the account authentication method: `oauth` and `setup-token` SHALL use subscription accounting, while `apikey` SHALL use metered accounting.

#### Scenario: OAuth account uses subscription accounting

- **WHEN** an OAuth account appears in the profit report
- **THEN** the report SHALL use its configured periodic fee and period days as the cost basis
- **AND THEN** it SHALL expose billing-window revenue and cash profit when expiry metadata is available

### Requirement: Subscription accounting uses discrete recharge cycles

The system SHALL record each subscription recharge as a discrete billing cycle with its own start date, fee, period days, currency, and notes. The system SHALL calculate each cycle end date as its start date plus its period days, and SHALL NOT assume automatic renewal after a cycle ends.

#### Scenario: Administrator records a recharge date

- **WHEN** an administrator records a 30-day subscription purchased on 2026-08-01
- **THEN** the system SHALL create a billing cycle starting on 2026-08-01
- **AND THEN** it SHALL calculate 2026-08-31 as that cycle's end date
- **AND THEN** the billing-window revenue and profit SHALL only include usage inside that cycle

#### Scenario: Subscription is not renewed immediately

- **WHEN** an account has a cycle from 2026-07-01 through 2026-07-31 and its next recharge starts on 2026-08-15
- **THEN** the system SHALL attribute no subscription cost from 2026-07-31 through 2026-08-15
- **AND THEN** it SHALL not merge the two cycles or assume renewal on 2026-07-31

#### Scenario: Grok quota resets inside one payment cycle

- **WHEN** a Grok subscription is paid on 2026-07-17 and expires on 2026-08-17
- **AND WHEN** its included monthly quota resets on 2026-08-01
- **THEN** the system SHALL keep one financial cycle covering 2026-07-17 through 2026-08-17
- **AND THEN** it SHALL not create a second purchase cost or split the financial cycle on 2026-08-01
- **AND THEN** the quota reset MAY increase available capacity and supply forecasts without changing profit cost

#### Scenario: Grok monthly reset is not a subscription expiry

- **WHEN** the Grok quota snapshot contains `billing_period_end`
- **THEN** the system SHALL use it only for quota display and capacity planning
- **AND THEN** it SHALL NOT use it to infer, create, or alter a financial recharge cycle

#### Scenario: Confirmed subscription cycle has zero purchase cost

- **WHEN** an administrator records an active subscription cycle with a purchase fee of `$0`
- **THEN** the system SHALL treat the cycle as confirmed and active
- **AND THEN** it SHALL report `$0` as the cycle purchase cost
- **AND THEN** cycle profit SHALL equal cycle revenue

#### Scenario: Subscription start date is not recorded

- **WHEN** a subscription account has no recorded recharge cycle but has `credentials.subscription_expires_at`
- **THEN** the system SHALL offer the derived dates as a legacy read-only fallback
- **AND THEN** it SHALL not persist or extend a cycle without administrator confirmation

#### Scenario: OAuth token expiry is not a subscription expiry

- **WHEN** a subscription account has `credentials.expires_at` but has neither a configured cycle start date nor `credentials.subscription_expires_at`
- **THEN** the system SHALL NOT derive a billing cycle from `credentials.expires_at`
- **AND THEN** the interface SHALL mark the billing-cycle view as requiring a configured start date

### Requirement: Date inference is an explicit unconfirmed draft

The system SHALL provide an administrator action to infer a possible subscription start date from available credential expiry metadata. The inferred date SHALL remain an unconfirmed form draft and SHALL NOT create a billing cycle or affect profit calculations until the administrator saves it as a recharge cycle.

#### Scenario: Subscription expiry is available

- **WHEN** an administrator requests a date inference and `credentials.subscription_expires_at` is available
- **THEN** the interface SHALL suggest a start date equal to that expiry minus the entered period days
- **AND THEN** it SHALL identify the source as subscription expiry

#### Scenario: Only OAuth token expiry is available

- **WHEN** an administrator requests a date inference and only `credentials.expires_at` is available
- **THEN** the interface MAY suggest an unconfirmed draft start date
- **AND THEN** it SHALL identify the source as OAuth token expiry and warn that the date is not a billing fact
- **AND THEN** it SHALL not affect profit calculations unless the administrator explicitly saves a recharge cycle

#### Scenario: API Key account uses metered accounting

- **WHEN** an API Key account appears in the profit report
- **THEN** the report SHALL calculate cost from the account-side cost stored in its usage logs
- **AND THEN** it SHALL not apply a periodic subscription fee

### Requirement: Metered profit uses upstream purchase discount

The system SHALL calculate API Key account cost from the raw upstream account-side model cost multiplied by the historical account billing multiplier stored with each usage record. The system SHALL calculate revenue from the user's actual charged amount independently.

#### Scenario: Upstream purchase price is discounted

- **WHEN** an API Key account has a historical account billing multiplier of `0.02` and a request has a raw upstream cost of `$100`
- **THEN** the system SHALL record `$2` as the request's account cost for profit calculation
- **AND THEN** it SHALL calculate profit by subtracting `$2` from the user's actual charged amount

#### Scenario: Customer selling multiplier differs from purchase discount

- **WHEN** a request is sold to a customer at a group multiplier of `0.10` while the account purchase multiplier is `0.02`
- **THEN** the system SHALL retain the user charge and account cost as separate values
- **AND THEN** the profit report SHALL not treat the `0.10` selling multiplier as the upstream account cost

### Requirement: Window efficiency baseline is automatically derived

The system SHALL derive the 5-hour window revenue baseline from the account's historical actual user charges and SHALL NOT require an administrator to enter a window baseline revenue value. This baseline SHALL be used only for the window-efficiency display and SHALL NOT affect revenue, cost, or profit calculations.

#### Scenario: Sufficient historical usage exists

- **WHEN** an account has historical charged usage spanning 5-hour intervals
- **THEN** the system SHALL use the highest revenue observed in a continuous 5-hour interval as the window-efficiency baseline
- **AND THEN** the cost configuration interface SHALL not present a manual baseline-revenue input

#### Scenario: Historical usage is insufficient

- **WHEN** an account has insufficient charged usage to derive a meaningful 5-hour baseline
- **THEN** the system SHALL omit the window-efficiency value
- **AND THEN** it SHALL not substitute a zero baseline or request a manual value

### Requirement: Manual cost type selection is unavailable

The system SHALL not present administrators with a manual subscription-versus-metered selector for an account cost configuration.

#### Scenario: Configuring an OAuth account

- **WHEN** an administrator opens cost configuration for an OAuth or Setup Token account
- **THEN** the interface SHALL identify it as subscription accounting
- **AND THEN** it SHALL allow configuration of periodic fee and period days

#### Scenario: Configuring an API Key account

- **WHEN** an administrator opens cost configuration for an API Key account
- **THEN** the interface SHALL identify it as metered accounting
- **AND THEN** it SHALL explain that historical account-side usage cost is used automatically

### Requirement: Existing configurations do not override authentication mapping

The system SHALL preserve existing configuration records but SHALL not let their stored cost type override the authentication-derived cost type.

#### Scenario: Existing conflicting API Key configuration

- **WHEN** an API Key account has a legacy configuration marked as subscription
- **THEN** the profit report SHALL use metered accounting
- **AND THEN** historical usage logs and user charges SHALL remain unchanged

### Requirement: Account financial statistics live in the account usage drawer

The system SHALL present account-level revenue, upstream cost, profit, and margin in the account-management usage drawer. For an active subscription cycle, the drawer SHALL present the full fixed purchase cost and cycle-to-date cash profit; it SHALL NOT present a changing amortized cost as the account's purchase cost. The global profit page SHALL present aggregate totals, trends, and a read-only per-account breakdown for the selected range, and SHALL label the range-dependent cost as amortized period cost.

#### Scenario: Administrator inspects an account

- **WHEN** an administrator opens an account's usage drawer and selects the statistics tab
- **THEN** the drawer SHALL display the account's revenue, cost, profit, margin, request count, and cost-accounting type for the selected period

#### Scenario: Subscription account has an active cycle

- **WHEN** a subscription account has a confirmed active cycle with a purchase fee of `$1200`
- **THEN** the drawer SHALL display `$1200` as the fixed current-cycle purchase cost regardless of elapsed days
- **AND THEN** current-cycle profit SHALL equal current-cycle revenue minus `$1200`

#### Scenario: Subscription account has no active cycle

- **WHEN** a subscription account has no confirmed active cycle
- **THEN** the drawer SHALL indicate that no active subscription cycle is available
- **AND THEN** it SHALL not display amortized period cost as a fixed purchase cost

#### Scenario: Administrator views global profit analysis

- **WHEN** an administrator opens the profit analysis page
- **THEN** the page SHALL display aggregate revenue, cost, profit, date filters, and an aggregate trend
- **AND THEN** it SHALL display read-only per-account revenue, amortized cost, profit, margin, and request totals for the same selected range
- **AND THEN** range-dependent cost SHALL be labelled as amortized period cost
- **AND THEN** it SHALL not render account-specific configuration actions

### Requirement: Profit APIs use bounded database queries

The system SHALL calculate global profit summary, daily trend, and per-account details without issuing database queries per account. The global profit page SHALL load these views through one overview request, while the existing summary and trend endpoints SHALL remain compatible. Batch configuration SHALL persist all eligible account configurations without one database round trip per account.

#### Scenario: Global profit page contains many subscription accounts

- **WHEN** an administrator loads the global profit page for a selected date range
- **THEN** the interface SHALL request summary, trend, and account details through one overview endpoint
- **AND THEN** the backend database query count SHALL remain constant as the number of subscription accounts increases
- **AND THEN** the returned revenue, metered cost, subscription amortization, profit, and margin SHALL equal the existing accounting formulas

#### Scenario: Existing clients call summary or trend

- **WHEN** an existing client calls the summary or trend endpoint directly
- **THEN** the endpoint SHALL preserve its response contract and accounting semantics
- **AND THEN** it SHALL use the same batch data-access path without per-account cycle queries

#### Scenario: Administrator batch-configures subscription accounts

- **WHEN** an administrator applies one subscription configuration to multiple eligible unconfigured accounts
- **THEN** the backend SHALL persist the eligible records using one batch repository operation
- **AND THEN** the operation SHALL not overwrite previously configured accounts
- **AND THEN** the response SHALL list only the accounts that were actually updated

### Requirement: Global profit overview uses a bounded-staleness snapshot

The system SHALL cache global profit overview responses for five minutes using the normalized date range and timezone as the cache key. A cache hit SHALL NOT execute profit database queries. Concurrent cache misses for the same key SHALL be collapsed into one load, and the cache SHALL retain only a bounded number of keys.

#### Scenario: Administrator reopens the same profit range

- **WHEN** an administrator requests the same profit date range and timezone within five minutes of a completed overview load
- **THEN** the system SHALL return the cached snapshot
- **AND THEN** it SHALL expose the snapshot generation time and cache-hit status
- **AND THEN** it SHALL not query the profit repositories again

#### Scenario: Multiple administrators request an uncached range concurrently

- **WHEN** concurrent overview requests use the same date range and timezone while no valid snapshot exists
- **THEN** the system SHALL execute one underlying overview load
- **AND THEN** all waiting requests SHALL receive that completed snapshot

#### Scenario: Financial accounting configuration changes

- **WHEN** an administrator creates, updates, batch-updates, or deletes a cost configuration or subscription cycle
- **THEN** the system SHALL invalidate cached global profit overviews after the write succeeds
- **AND THEN** the next overview request SHALL recompute the affected financial result

#### Scenario: Administrator explicitly refreshes the snapshot

- **WHEN** an administrator requests a manual refresh for the selected profit range
- **THEN** the system SHALL bypass the existing cached value
- **AND THEN** it SHALL replace that key with a newly generated snapshot

#### Scenario: Global overview omits drawer-only enrichment

- **WHEN** the backend builds a global profit overview
- **THEN** it SHALL calculate range revenue, metered cost, subscription amortization, profit, margin, requests, and trend
- **AND THEN** it SHALL not query the historical best 5-hour window or current-cycle revenue used only by the account drawer
- **AND THEN** the account drawer SHALL continue to return those account-specific operational metrics
