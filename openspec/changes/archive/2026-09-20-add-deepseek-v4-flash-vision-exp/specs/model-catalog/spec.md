## ADDED Requirements

### Requirement: DeepSeek V4 Flash Vision Exp availability

The system SHALL expose `deepseek-v4-flash-vision-exp` as a DeepSeek chat model, preserve the requested model ID when routing, and mark the model as vision-capable for model selection and billing metadata.

#### Scenario: Model appears in catalog

- **WHEN** the first-party model catalog is loaded
- **THEN** the DeepSeek platform model list contains `deepseek-v4-flash-vision-exp` with a self-mapping and vision capability metadata

#### Scenario: Model is selectable in the client

- **WHEN** the frontend builds the DeepSeek model whitelist and presets
- **THEN** `deepseek-v4-flash-vision-exp` is available as a selectable DeepSeek model and preset
