## ADDED Requirements

### Requirement: DeepSeek V4.1 Flash 1017 pricing parity

The system SHALL resolve `deepseek-v4.1-flash-1017` at the official DeepSeek V4.1 Flash
CNY rates instead of leaving it unpriced. Peak requests (Monday–Friday, 01:00–04:00 and
06:00–10:00 UTC) SHALL use 2.00 CNY per million input tokens, 2.00 CNY per million cache
write tokens, 0.04 CNY per million cache hit tokens, and 8.00 CNY per million output
tokens. Off-peak requests SHALL use half of each rate. Pricing currency SHALL remain
`CNY` and SHALL NOT be converted during settlement.

#### Scenario: Suffixed ID resolves the official entry

- **WHEN** billing resolves pricing for `deepseek-v4.1-flash-1017`
- **THEN** it returns the official V4.1 Flash CNY entry rather than a missing-pricing error

#### Scenario: Peak window applies official rates

- **WHEN** `deepseek-v4.1-flash-1017` is billed inside an official peak window
- **THEN** one million input, cache-write, cache-hit, and output tokens cost 2.00, 2.00, 0.04, and 8.00 CNY respectively

#### Scenario: Off-peak window halves the rates

- **WHEN** `deepseek-v4.1-flash-1017` is billed outside the peak windows
- **THEN** the input, cache-write, cache-hit, and output costs are exactly half of the peak amounts

#### Scenario: Unrelated spellings keep their own prices

- **WHEN** pricing is resolved for `deepseek-v4.1-flash`, `deepseek-v4-flash`, or `deepseek-v4-pro`
- **THEN** each keeps the rates it had before this change

#### Scenario: Other vendor suffixes stay fail-closed

- **WHEN** billing resolves an unpriced vendor suffix such as `deepseek-v4.1-flash-uncensored`
- **THEN** the request is rejected by pricing admission as unavailable rather than settled at zero cost
