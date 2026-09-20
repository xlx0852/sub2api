# usage-billing-core Specification

## Purpose
将各平台用量计算与日志构造归入共享结算核心，同时以显式策略保留平台计费差异和幂等语义。 本规范用于约束相关实现、回归测试与后续变更评审，具体可观测行为以下列场景为准。

## Requirements

### Requirement: All gateway platforms use one usage billing core

The system SHALL calculate billable usage, construct the usage log, and hand the result to durable or legacy persistence through one shared billing core.

#### Scenario: Gateway and OpenAI requests reach settlement

- **WHEN** equivalent Gateway or OpenAI provider usage is submitted for billing
- **THEN** both requests use the same calculation and usage-log construction implementation before the existing persistence mode is selected

#### Scenario: A new platform usage field is introduced

- **WHEN** a platform adds an optional settlement field
- **THEN** the field is represented explicitly in the shared input or policy rather than by copying a second calculator or log builder

### Requirement: Platform-specific billing semantics remain explicit

The system SHALL preserve existing platform differences through named policies and SHALL NOT silently normalize different pricing or token-bucket behavior.

#### Scenario: Pricing is missing

- **WHEN** model pricing is missing for an OpenAI settlement
- **THEN** the existing zero-cost behavior is preserved
- **AND** an equivalent Gateway settlement retains its existing logged-error plus zero-cost usage behavior

#### Scenario: Long-context billing is evaluated

- **WHEN** a Gateway Gemini request or an OpenAI account crosses its existing long-context condition
- **THEN** Gateway uses its fixed threshold/multiplier while OpenAI uses the existing account capability switch

#### Scenario: Cache usage is normalized

- **WHEN** OpenAI usage contains cached or cache-creation tokens
- **THEN** its existing mutually exclusive input buckets are preserved
- **AND** Gateway `ForceCacheBilling` retains its existing behavior

#### Scenario: OpenAI-only attribution is present

- **WHEN** a settlement contains WS, compact, service-tier, video, or cyber-blocked attribution
- **THEN** the shared usage log stores the same fields and request type as the pre-merge OpenAI path

### Requirement: Billing-core consolidation is behavior preserving

The system SHALL preserve protocol I/O, calculated token and cost fields, multipliers, balance/quota effects, Profit contribution, scheduling behavior, configuration keys, and deployment defaults.

#### Scenario: Old and consolidated implementations process the same fixture

- **WHEN** the differential harness sends identical HTTP, SSE, disconnect, WS, or short-path fixtures to the baseline and consolidated builds
- **THEN** response schemas, canonical usage, persisted billing fields, and balance/quota deltas are equal with every difference explicitly attributed
