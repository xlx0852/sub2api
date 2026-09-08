## ADDED Requirements

### Requirement: Qwen and MiMo official model pricing

The system SHALL provide exact official Chinese pricing in CNY for Qwen models (`qwen3.7-max`, `qwen3.8-flash`, `qwen3.8-max`) and Xiaomi MiMo models (`mimo-v2.5`, `mimo-v2.5-pro`). Exact model IDs SHALL be resolved in both dynamic pricing and billing fallback.

#### Scenario: Qwen 3.8 Max pricing resolution
- **WHEN** `qwen3.8-max` is queried for pricing
- **THEN** input cost is 12 CNY per million tokens, output cost is 36 CNY per million tokens, cache read is 2.4 CNY per million tokens, and cache write is 17.5 CNY per million tokens

#### Scenario: Qwen 3.8 Flash pricing resolution
- **WHEN** `qwen3.8-flash` is queried for pricing
- **THEN** input cost is 0.80 CNY per million tokens, output cost is 2.70 CNY per million tokens, cache read is 0.08 CNY per million tokens, and cache write is 0.01 CNY per million tokens

#### Scenario: Qwen 3.7 Max pricing resolution
- **WHEN** `qwen3.7-max` is queried for pricing
- **THEN** input cost is 12 CNY per million tokens, output cost is 36 CNY per million tokens, cache read is 2.4 CNY per million tokens, and cache write is 17.5 CNY per million tokens

#### Scenario: MiMo 2.5 Pro pricing resolution
- **WHEN** `mimo-v2.5-pro` is queried for pricing
- **THEN** input cost is 3.0 CNY per million tokens, output cost is 6.0 CNY per million tokens, and cache read is 0.025 CNY per million tokens

#### Scenario: MiMo 2.5 pricing resolution
- **WHEN** `mimo-v2.5` is queried for pricing
- **THEN** input cost is 1.0 CNY per million tokens, output cost is 2.0 CNY per million tokens, and cache read is 0.02 CNY per million tokens

#### Scenario: Unknown Qwen or MiMo alias rejection
- **WHEN** an unconfigured variant such as `qwen-max` or `mimo` is queried
- **THEN** the system SHALL NOT automatically fall back and SHALL return ErrModelPricingUnavailable unless explicitly configured
