## ADDED Requirements

### Requirement: Grok text rate card follows the official xAI price list

The system SHALL price Grok text models from xAI's published rates, including the short-context and
long-context tiers, the cached-input rate, and the 2x Priority rate. The long-context threshold SHALL
be 200,000 prompt tokens for every Grok text model that declares a long-context tier, and once a
request reaches that threshold the whole request SHALL be billed at the long-context rates.

#### Scenario: Short-context rates resolve per model

- **WHEN** a Grok text request stays below the long-context threshold
- **THEN** input, cached input and output are billed at the model's short-context rates, for example `grok-4.3` at $1.25 / $0.20 / $2.50 and `grok-4.5` cached input at $0.30 per million tokens

#### Scenario: Long-context rates apply to the whole request

- **WHEN** a Grok text request's total prompt tokens exceed 200,000
- **THEN** every token in that request is billed with a 2x input-side multiplier and a 2x output multiplier

#### Scenario: Priority is exactly twice standard

- **WHEN** a Grok text request is served at the priority tier
- **THEN** its input, cached input and output are billed at twice the standard rates, for example `grok-4.3` at $2.50 / $0.40 / $5.00

#### Scenario: Every served Grok text model has a price

- **WHEN** pricing is resolved for `grok-4.20-0309-reasoning`, `grok-4.20-0309-non-reasoning` or `grok-4.20-multi-agent-0309`
- **THEN** a price is found rather than falling through to no-pricing, and the alias spellings `grok-4.20-reasoning` and `grok-4.20-non-reasoning` resolve to the same rates

### Requirement: Grok Imagine image pricing covers the published model line

The system SHALL price `grok-imagine-image-2.0` in addition to the existing Imagine image models, and
SHALL keep the per-resolution tiers of the other Imagine models aligned with xAI's published rates.

#### Scenario: Image 2.0 resolves a per-resolution price

- **WHEN** `grok-imagine-image-2.0` is billed for a 1K and a 2K image
- **THEN** the 1K tier resolves to the model's default-quality rate and the 2K tier to the corresponding higher rate, instead of falling through to the generic image default

#### Scenario: Existing Imagine tiers are unchanged

- **WHEN** `grok-imagine-image`, `grok-imagine-image-quality`, `grok-imagine-video` or `grok-imagine-video-1.5` are billed
- **THEN** their per-image and per-second rates remain at xAI's published values

### Requirement: Grok rate-card coverage gaps are recorded

Pricing surfaces that xAI publishes but the gateway does not yet model SHALL be recorded with their
official values so they are not mistaken for verified coverage.

#### Scenario: Gaps are discoverable

- **WHEN** a reviewer inspects the Grok rate-card change
- **THEN** the deferred surfaces list their official rates: server-side tool invocations, the Batch discount, the US regional endpoint uplift, input-image and input-video charges, voice, and storage
