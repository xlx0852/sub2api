## ADDED Requirements

### Requirement: Model pricing carries an explicit source currency

The system SHALL associate each model pricing entry with a normalized `USD` or `CNY` source currency. Missing legacy currency metadata SHALL be interpreted as `USD` for compatibility, and one pricing entry SHALL NOT mix currencies across its token, request, image, video, or interval prices.

#### Scenario: USD model uses legacy pricing

- **WHEN** a legacy model price has no explicit currency
- **THEN** the system SHALL resolve its source currency as `USD`
- **AND THEN** its numeric prices and customer selling multiplier SHALL remain unchanged

#### Scenario: Administrator configures CNY pricing

- **WHEN** an administrator saves a channel pricing entry with currency `CNY`
- **THEN** every price in that entry and its intervals SHALL be interpreted as CNY source-price numbers
- **AND THEN** the system SHALL persist and return `CNY` in the pricing contract

#### Scenario: Unsupported currency is submitted

- **WHEN** a pricing entry specifies a currency other than `USD` or `CNY`
- **THEN** the system SHALL reject the configuration
- **AND THEN** it SHALL NOT silently convert or relabel the prices

### Requirement: Platform credits are currency-neutral

The system SHALL treat user balances, actual user charges, API Key quotas, account metered costs, and calculated profit as currency-neutral platform credits. The system SHALL calculate a user charge as the numeric model base price multiplied by the applicable sales multiplier and SHALL NOT automatically convert the model pricing currency.

#### Scenario: Customer uses a USD-priced Codex model

- **WHEN** a Codex model has a USD numeric base price and the group sales multiplier is `0.12`
- **THEN** the system SHALL charge `base numeric price × 0.12` platform credits
- **AND THEN** it SHALL NOT multiply the charge by a USD/CNY exchange rate

#### Scenario: Customer uses a CNY-priced DeepSeek model

- **WHEN** a DeepSeek model has a CNY numeric base price and the group sales multiplier is `0.30`
- **THEN** the system SHALL charge `base numeric price × 0.30` platform credits
- **AND THEN** the user's balance unit and recharge ratio SHALL remain unchanged

### Requirement: Account metered cost uses a manually controlled comprehensive multiplier

The system SHALL calculate API Key account metered cost as the account-side numeric base cost multiplied by the historical comprehensive cost multiplier snapshot. The platform SHALL NOT fetch, derive, store, or apply an automatic foreign-exchange rate to this calculation.

#### Scenario: Operator includes exchange rate and fees

- **WHEN** an operator configures an account comprehensive cost multiplier that includes upstream discount, exchange rate, fees and risk buffer
- **THEN** the account cost SHALL equal the account-side numeric base cost multiplied by that configured value
- **AND THEN** the platform SHALL NOT decompose or override the configured value

#### Scenario: Account multiplier changes later

- **WHEN** an operator changes the current account comprehensive cost multiplier
- **THEN** new usage SHALL snapshot and use the new multiplier
- **AND THEN** historical usage and historical profit SHALL continue using their original snapshots

### Requirement: Usage preserves pricing currency and billing snapshots

The system SHALL snapshot the resolved pricing currency, numeric base costs, customer sales multiplier, actual customer charge, and account comprehensive cost multiplier for each new usage record. It SHALL preserve historical usage values without retroactive currency conversion or recalculation.

#### Scenario: New CNY usage is settled asynchronously

- **WHEN** a CNY-priced request reaches terminal usage and is handed to durable settlement
- **THEN** the settlement payload and resulting usage log SHALL preserve `CNY` together with the original numeric costs and multipliers
- **AND THEN** a retry SHALL NOT resolve currency from newer pricing configuration

#### Scenario: Historical usage has no currency snapshot

- **WHEN** an administrator views a historical usage row written before currency snapshots existed
- **THEN** the interface MAY display its base-price currency as legacy `USD`
- **AND THEN** the system SHALL NOT rewrite its costs, customer charge, or multipliers

### Requirement: Base currency and settlement-value presentation are distinct

The system SHALL always display model base prices and raw base-cost components using the pricing source currency symbol. Missing or legacy source currency SHALL render as USD with `$`, and a model base price SHALL never render with an empty unit. Customer charges, balances, quotas, account costs, and profit SHALL be displayed as plain numbers without `$` or `¥` symbols and without an appended credit unit label.

#### Scenario: Administrator views DeepSeek cost details

- **WHEN** a DeepSeek usage row has pricing currency `CNY`
- **THEN** its input price, output price, component base costs, and raw base total SHALL display `¥`
- **AND THEN** its user charge SHALL display as a plain number without a currency symbol or unit label
- **AND THEN** its account cost SHALL display as a plain number without a currency symbol or unit label

#### Scenario: Administrator views Codex cost details

- **WHEN** a Codex usage row has pricing currency `USD`
- **THEN** its input price, output price, component base costs, and raw base total SHALL display `$`
- **AND THEN** its user charge and account cost SHALL display as plain numbers without currency symbols or unit labels

#### Scenario: Legacy model has no currency metadata

- **WHEN** a model base price or historical raw cost has no explicit pricing currency
- **THEN** the interface SHALL display `$` as its base-price unit
- **AND THEN** it SHALL NOT render an empty unit

### Requirement: DeepSeek uses official Chinese peak and off-peak pricing

The system SHALL price DeepSeek V4 Flash, DeepSeek V4 Pro, and their supported official aliases using the Chinese official CNY price table. The system SHALL apply peak pricing during Beijing time `09:00-12:00` and `14:00-18:00`, and half-price off-peak pricing at all other times.

#### Scenario: DeepSeek Flash runs during peak hours

- **WHEN** a DeepSeek V4 Flash request is priced during a Beijing peak interval
- **THEN** cached input SHALL use `¥0.10` per million tokens
- **AND THEN** uncached input SHALL use `¥3.00` per million tokens
- **AND THEN** output SHALL use `¥9.00` per million tokens

#### Scenario: DeepSeek Pro runs outside peak hours

- **WHEN** a DeepSeek V4 Pro request is priced outside Beijing peak intervals
- **THEN** cached input SHALL use `¥0.15` per million tokens
- **AND THEN** uncached input SHALL use `¥4.50` per million tokens
- **AND THEN** output SHALL use `¥13.50` per million tokens

### Requirement: Existing balances and historical billing remain unchanged

The system SHALL introduce mixed-currency model pricing without migrating existing user balances, recharge ratios, API Key quotas, Codex selling multipliers, or historical usage amounts.

#### Scenario: Mixed-currency pricing is deployed

- **WHEN** the new pricing metadata and presentation are deployed
- **THEN** existing balances, recharge principal records, quotas and historical usage values SHALL be byte-for-byte numerically unchanged
- **AND THEN** Codex/OpenAI requests using unchanged pricing and multipliers SHALL produce the same numeric customer charges as before
