## ADDED Requirements

### Requirement: Forecast exposes spendable stored-value demand

The system SHALL report the total positive spendable balance of active, non-deleted, non-admin users as stored-value demand. Frozen balance SHALL be reported separately and SHALL NOT be included in spendable demand.

#### Scenario: Users have positive, negative, and frozen balances

- **WHEN** the administrator opens the supply forecast
- **THEN** the system SHALL sum only positive spendable balances from eligible users
- **AND THEN** it SHALL not reduce that sum with another user's negative balance
- **AND THEN** it SHALL show frozen balance separately without treating it as immediately consumable demand

### Requirement: Forecast burn rate uses balance-billed usage only

The system SHALL calculate stored-value burn rate from successful positive user charges that were not covered by a user subscription. It SHALL expose both 7-day and 30-day daily averages and SHALL use the greater average as the base planning rate.

#### Scenario: User has balance and subscription usage

- **WHEN** recent usage contains both balance-billed rows and rows linked to an active user subscription
- **THEN** the burn rate SHALL include the balance-billed actual charges
- **AND THEN** it SHALL exclude subscription-linked usage from stored-value depletion

### Requirement: Forecast exposes horizon and safety assumptions

The system SHALL support 7, 30, 60, and 90-day planning horizons and a bounded safety margin. It SHALL return the generated time, history windows, selected horizon, selected margin, base daily demand, planning daily demand, projected consumption, and stored-value runway.

#### Scenario: Administrator plans for 30 days

- **WHEN** the administrator selects a 30-day horizon and a 20 percent safety margin
- **THEN** projected consumption SHALL not exceed the current spendable balance
- **AND THEN** supply demand SHALL use the base daily demand multiplied by 1.20
- **AND THEN** the interface SHALL disclose those assumptions next to the result

#### Scenario: There is no recent balance consumption

- **WHEN** both the 7-day and 30-day balance-billed averages are zero
- **THEN** the system SHALL report that runway and future supply cannot be estimated from current history
- **AND THEN** it SHALL not report zero required accounts as if that were a confident forecast

### Requirement: Demand is allocated by observed platform mix

The system SHALL allocate future demand by each platform's share of recent balance-billed actual charges. If no platform history exists, the system SHALL omit platform account requirements and explain that the demand mix is unavailable.

#### Scenario: Recent spend is split across platforms

- **WHEN** 70 percent of recent balance-billed charges used OpenAI accounts and 30 percent used Grok accounts
- **THEN** the platform forecast SHALL allocate 70 percent of planning demand to OpenAI and 30 percent to Grok

### Requirement: Subscription supply uses observed account capacity

For OAuth and Setup Token supply, the system SHALL derive observed account capacity from the 75th percentile of positive account-day actual charges in the recent 30-day sample. It SHALL calculate required accounts from platform planning demand, compare the result with deduplicated currently schedulable subscription accounts, and expose the resulting shortage or surplus with sample size and confidence.

#### Scenario: Platform has sufficient subscription samples

- **WHEN** a platform has a positive P75 account-day capacity and planning daily demand
- **THEN** required accounts SHALL equal the ceiling of planning daily demand divided by P75 account-day capacity
- **AND THEN** current supply SHALL count each schedulable subscription account once even when it belongs to multiple groups
- **AND THEN** shortage SHALL equal the positive difference between required and current accounts

#### Scenario: Platform lacks subscription capacity samples

- **WHEN** a platform has projected subscription demand but no positive account-day sample
- **THEN** the system SHALL mark subscription account demand as unavailable
- **AND THEN** it SHALL expose the missing sample reason instead of returning zero required accounts

### Requirement: Metered supply reports procurement budget

For API Key supply, the system SHALL estimate upstream procurement budget from the historical metered-cost-to-revenue ratio for the same platform. It SHALL NOT convert that amount into a required account count without a separately defined capacity limit.

#### Scenario: Metered platform has historical cost ratio

- **WHEN** API Key routes produced `$1000` of customer charges and `$200` of upstream cost in the sample window
- **THEN** the system SHALL use `0.20` as the procurement-cost ratio
- **AND THEN** it SHALL estimate the planning-period upstream budget by multiplying projected metered customer demand by `0.20`

### Requirement: Mixed platform supply remains separated

When a platform has both subscription and API Key historical routes, the system SHALL split projected demand by their observed customer-charge shares. It SHALL report subscription account requirements and metered procurement budget separately.

#### Scenario: Platform uses both supply types

- **WHEN** recent OpenAI balance demand was served 80 percent by subscription accounts and 20 percent by API Key accounts
- **THEN** the system SHALL apply account-capacity forecasting to the 80 percent subscription share
- **AND THEN** it SHALL apply procurement-budget forecasting to the 20 percent metered share

### Requirement: Forecast is lazy-loaded and cached

The supply forecast SHALL load only after the administrator opens its profit-analysis tab. The backend SHALL cache each normalized horizon, safety-margin, and timezone snapshot for 15 minutes, collapse concurrent misses, expose ETag and cache status, and support manual refresh.

#### Scenario: Administrator opens profit review only

- **WHEN** the administrator opens the profit analysis page and remains on the profit-review tab
- **THEN** the frontend SHALL not request the supply-forecast endpoint
- **AND THEN** existing profit overview loading SHALL not wait for forecast computation

#### Scenario: Administrator reopens the same forecast

- **WHEN** the same normalized forecast is requested within 15 minutes
- **THEN** the backend SHALL return the cached snapshot without rerunning forecast repository queries

### Requirement: Forecast confidence and limitations are explicit

The system SHALL expose sample account-days, active sample accounts, confidence, and unavailability reasons. The interface SHALL identify the forecast as an operational estimate based on historical realized demand and capacity, not an official upstream quota or accounting liability statement.

#### Scenario: Forecast is based on a small sample

- **WHEN** a platform forecast has fewer than the configured minimum sample accounts or active account-days
- **THEN** the interface SHALL label the result as low confidence
- **AND THEN** it SHALL keep the sample counts visible with the estimate
