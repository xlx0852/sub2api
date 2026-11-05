## ADDED Requirements

### Requirement: GPT-5.5 family pricing in the catalog fallback layer

The catalog `fallback_pricing` block SHALL price `gpt-5.5` and `gpt-5.5-pro` from OpenAI's official
rates as self-contained entries, and SHALL NOT change any other entry.

#### Scenario: Standard rates for gpt-5.5

- **WHEN** `gpt-5.5` is resolved through the catalog fallback pricing block
- **THEN** the input cost is 5 USD per million tokens, the cache read cost is 0.5 USD per million tokens, the output cost is 30 USD per million tokens, and the long-context tier is 272,000 input tokens with a 2x input and 1.5x output multiplier

#### Scenario: Standard rates for gpt-5.5-pro

- **WHEN** `gpt-5.5-pro` is resolved through the catalog fallback pricing block
- **THEN** the input cost is 30 USD per million tokens, the cache read cost is 3 USD per million tokens, the output cost is 180 USD per million tokens, and the long-context tier is 272,000 input tokens with a 2x input and 1.5x output multiplier

#### Scenario: No cache-write price is declared

- **WHEN** the GPT-5.5 entries are inspected
- **THEN** neither entry declares a cache creation price nor cache breakdown support, matching the official table's `-` for cache writes

#### Scenario: Entries are self-contained

- **WHEN** the catalog fallback pricing block is validated
- **THEN** neither `gpt-5.5` nor `gpt-5.5-pro` carries an `alias_of` field, so no GPT-5.4 rate can be inherited

### Requirement: GPT-5.5 family Fast tier pricing

The system SHALL price `gpt-5.5` Fast (priority) requests from the declared priority unit prices, and
SHALL charge `gpt-5.5-pro` standard rates on that tier because OpenAI publishes no Fast tier for it.

#### Scenario: Fast tier uses the declared priority prices

- **WHEN** a `gpt-5.5` request is billed with `service_tier` `priority`
- **THEN** the input cost is 12.5 USD per million tokens, the cache read cost is 1.25 USD per million tokens, and the output cost is 75 USD per million tokens

#### Scenario: Standard tier stays at standard rates

- **WHEN** a `gpt-5.5` request is billed without a service tier
- **THEN** the input, cache read, and output costs are the standard 5 / 0.5 / 30 USD per million tokens

#### Scenario: No Fast tier for the pro model

- **WHEN** a `gpt-5.5-pro` request is billed with `service_tier` `priority`
- **THEN** the standard 30 / 3 / 180 USD per million token rates apply with no priority premium

### Requirement: GPT-5.5 family static fallback pricing

The pricing service SHALL resolve official GPT-5.5 family rates from static fallback constants so that
billing is correct even when no pricing data file is loaded, and SHALL match the `gpt-5.5-pro` prefix
ahead of the `gpt-5.5` prefix.

#### Scenario: Static fallback for gpt-5.5

- **WHEN** the pricing service has no pricing data loaded and `gpt-5.5` is queried
- **THEN** the official GPT-5.5 rates are returned instead of the GPT-5.4 rates or the default test model price

#### Scenario: Pro is not swallowed by the base branch

- **WHEN** the pricing service has no pricing data loaded and `gpt-5.5-pro` is queried
- **THEN** the GPT-5.5 Pro rates (30 USD input, 3 USD cache read, 180 USD output per million tokens) are returned

#### Scenario: Variants inherit the family price

- **WHEN** a model ID such as `gpt-5.5-2026-04-23` or `gpt-5.5-medium` is queried with no pricing data loaded
- **THEN** it resolves to the GPT-5.5 family rates rather than the GPT-5.4 rates
