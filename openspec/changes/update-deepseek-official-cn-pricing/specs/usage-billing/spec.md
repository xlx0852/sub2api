## MODIFIED Requirements

### Requirement: DeepSeek official Chinese pricing

The system SHALL calculate DeepSeek V4 charges in CNY using the official Chinese price bands: Flash and Flash Vision Exp SHALL use 3 CNY per million uncached input tokens, 9 CNY per million output tokens, and 0.10 CNY per million cached input tokens at peak; Pro SHALL use 9, 27, and 0.30 respectively. Requests in the official UTC peak windows on weekdays SHALL use these peak prices, while off-peak requests SHALL use half these prices.

#### Scenario: Flash Vision Exp peak pricing

- **WHEN** `deepseek-v4-flash-vision-exp` is billed during 01:00–04:00 or 06:00–10:00 UTC
- **THEN** its input, output, and cache-hit rates are 3, 9, and 0.10 CNY per million tokens respectively

#### Scenario: Flash Vision Exp off-peak pricing

- **WHEN** `deepseek-v4-flash-vision-exp` is billed outside those windows
- **THEN** its input, output, and cache-hit rates are 1.5, 4.5, and 0.05 CNY per million tokens respectively
