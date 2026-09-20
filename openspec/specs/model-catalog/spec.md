# model-catalog Specification

## Purpose
维护已登记模型的可用性、官方定价和客户端映射，使未知别名不能意外继承定价。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: DeepSeek V4 Flash Vision Exp availability

The system SHALL expose `deepseek-v4-flash-vision-exp` as a DeepSeek chat model, preserve the requested model ID when routing, and mark the model as vision-capable for model selection and billing metadata.

#### Scenario: Model appears in catalog

- **WHEN** the first-party model catalog is loaded
- **THEN** the DeepSeek platform model list contains `deepseek-v4-flash-vision-exp` with a self-mapping and vision capability metadata

#### Scenario: Model is selectable in the client

- **WHEN** the frontend builds the DeepSeek model whitelist and presets
- **THEN** `deepseek-v4-flash-vision-exp` is available as a selectable DeepSeek model and preset

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
