## ADDED Requirements

### Requirement: Cost basis is separate from sell price

The system SHALL store a vendor cost basis per model pattern (currency, input / cached /
output price per million tokens, discount percentage, observation time, source) and SHALL
use it only to derive sell prices and report margin. The charge path SHALL NOT read the
cost basis directly.

#### Scenario: Cost basis does not affect a manual price

- **WHEN** a price row is `manual` and its cost basis is updated
- **THEN** the charged price SHALL remain exactly the authored value

#### Scenario: Margin is reported from cost basis

- **WHEN** an operator opens the group pricing summary for a model with a cost basis
- **THEN** the response SHALL include cost, derived sell price, effective price after the group multiplier, and the resulting gross margin

### Requirement: Sell price is derived per row with an explicit FX mode

Every price row SHALL declare one mode: `cny_native` (priced in CNY, never converted),
`fx_auto` (derived from the cost basis as `cost × (1 − discount) × fx_rate × (1 + markup)`),
or `manual` (authored CNY value, never converted). The `currency` column SHALL be display
metadata only and SHALL NOT influence the charged amount.

#### Scenario: USD model is converted

- **WHEN** an `fx_auto` row with cost `$0.80` per million input tokens, no discount, no markup and `fx_rate = 7.0` is charged
- **THEN** the sell price SHALL be `¥5.60` per million input tokens

#### Scenario: Native CNY model is not converted

- **WHEN** a `cny_native` row priced at `¥2.00` per million input tokens is charged after the FX rate changes
- **THEN** the charged price SHALL remain `¥2.00`

#### Scenario: Manual row survives FX changes

- **WHEN** the configured FX rate changes and some rows are `manual`
- **THEN** those rows SHALL keep their authored price and their existing charges SHALL remain reproducible with the recorded `fx_rate` of the original charge

#### Scenario: USD-labelled row cannot charge at par

- **WHEN** a price row whose currency metadata is `USD` is used for charging
- **THEN** the charged amount SHALL be the row's CNY sell price and SHALL NOT be the raw USD number

### Requirement: One price source per model

Sell prices SHALL come from exactly two places with a single precedence: the group-bound
sell policy row first, then the built-in defaults document in the same schema. Code-level
price pins, a second snapshot file, and implicit family fallbacks outside that document
SHALL NOT exist.

#### Scenario: Policy row wins

- **WHEN** a group has a sell policy row for a model
- **THEN** that row's price SHALL be used and the built-in default SHALL be ignored

#### Scenario: Unknown model fails closed

- **WHEN** a model resolves to neither a policy row nor a built-in default
- **THEN** the request SHALL be rejected with `503 PRICING_UNAVAILABLE`

#### Scenario: Official domestic prices keep resolving

- **WHEN** a configured exact model such as `qwen3.8-max` or `deepseek-v4.1-flash` is priced
- **THEN** its built-in default value SHALL match the previously specified official CNY price

### Requirement: Context tiers replace intervals and long-context multipliers

Price rows SHALL express context-dependent pricing as one ordered tier list where each tier
declares a token range plus either absolute prices or a multiplier. The system SHALL NOT
apply a second, separate long-context multiplier mechanism.

#### Scenario: Tier selection

- **WHEN** a request's context size falls inside a declared tier
- **THEN** that tier's prices SHALL be used for the whole request and no other context multiplier SHALL be applied

#### Scenario: Single tier behaves as before

- **WHEN** a model has exactly one open-ended tier
- **THEN** its price SHALL equal the flat row price with no context-dependent change

### Requirement: Modifiers are data-driven and auditable

Time-window and request-tier adjustments SHALL be expressed as modifier records with an
explicit scope, target, window or tier value, multiplier and enabled flag, and SHALL be
applied in a fixed documented order. A vendor peak window SHALL be disableable per model
pattern without introducing a price row.

#### Scenario: Vendor off-peak discount is configurable

- **WHEN** the DeepSeek off-peak modifier is enabled with `outside` mode over the weekday peak windows at `0.5`
- **THEN** off-peak requests SHALL be charged half of the peak price and peak requests SHALL be charged the full price

#### Scenario: Peak pricing can be turned off

- **WHEN** the operator disables the vendor-time modifier for a model pattern
- **THEN** every request for that pattern SHALL be charged the flat price at any hour

#### Scenario: Channel price bypasses vendor windows

- **WHEN** a request is charged from a sell policy row
- **THEN** vendor-time modifiers SHALL NOT change its price, and any fixed-price intent SHALL remain intact

#### Scenario: Modifier audit

- **WHEN** a usage row is written
- **THEN** it SHALL record the applied modifier identifiers, the FX mode and the FX rate used

### Requirement: Group rate and balance denomination are unchanged

The group rate multiplier, per-user override and balance denomination SHALL keep their
current semantics: the effective customer price is the sell price multiplied by the group
rate and divided by the balance unit value, with one balance unit credited per one unit of
recharge principal.

#### Scenario: Effective price composition

- **WHEN** a sell price is `¥5.60` per million tokens, the group rate is `1.5` and one balance unit equals `¥1`
- **THEN** the customer SHALL be charged `8.40` balance units per million tokens

### Requirement: Migration preserves every existing charge

The migration SHALL preserve the charged value of every existing price row: rows start as
`manual`, rows imported from a known cost basis are switched to `fx_auto` only after a
verification that the derived price equals the previous value within `1e-9`, and the
migration SHALL abort instead of writing a mismatch.

#### Scenario: Existing row is unchanged

- **WHEN** an existing price row is migrated
- **THEN** its sell price and any already-recorded usage charge SHALL be identical to the pre-migration value

#### Scenario: Shadow comparison is clean before cutover

- **WHEN** the unified pipeline runs in shadow mode
- **THEN** it SHALL log per-request deltas against the legacy pipeline without affecting the charge, and cutover SHALL require zero deltas beyond rounding

#### Scenario: Rollback

- **WHEN** the pipeline switch is set back to `legacy`
- **THEN** charging SHALL return to the previous code path without a data rollback
