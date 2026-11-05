## ADDED Requirements

### Requirement: Kimi K3.1 Pro catalog entry

The system SHALL expose `kimi-k3.1-pro` as a first-class Kimi chat model with the display name
`Kimi K3.1 Pro`. Its metadata SHALL declare reasoning support, image and video input support, and a
1,048,576-token context window. The catalog SHALL provide a self mapping, a `kimi3.1` compatibility
alias, and a `Kimi K3.1 Pro` client preset without changing the existing Kimi models.

#### Scenario: Canonical model is discoverable

- **WHEN** the first-party model catalog is loaded
- **THEN** the Kimi platform contains canonical model `kimi-k3.1-pro` with display name `Kimi K3.1 Pro`, reasoning enabled, image/video input enabled, and a 1,048,576-token context window

#### Scenario: Compatibility entry is selectable

- **WHEN** a client resolves the Kimi compatibility name `kimi3.1`
- **THEN** it resolves to `kimi-k3.1-pro`, and the Kimi UI presets include `Kimi K3.1 Pro`

#### Scenario: Existing Kimi catalog entries remain available

- **WHEN** the catalog is loaded after K3.1 Pro is added
- **THEN** `k3`, `k3-256k`, `kimi-for-coding`, and `kimi-for-coding-highspeed` retain their existing metadata, aliases, and presets

### Requirement: Kimi K3.1 Pro official pricing

The system SHALL resolve exact official CNY prices for `kimi-k3.1-pro` and its compatibility spellings
before considering the broad legacy `kimi-k3` fallback. Standard service SHALL use input `13e-6`, cache
read `1.30e-6`, cache write `13e-6`, and output `68e-6` per token. The existing priority service-tier
fields SHALL represent Highspeed with input `26e-6`, cache read `1.30e-6`, cache write `26e-6`, and
output `136e-6` per token.

#### Scenario: Standard pricing resolves

- **WHEN** `kimi-k3.1-pro` is priced for a standard service-tier request
- **THEN** input, cache-read, cache-write, and output costs resolve to `13e-6`, `1.30e-6`, `13e-6`, and `68e-6` CNY per token respectively

#### Scenario: Highspeed pricing resolves through priority fields

- **WHEN** `kimi-k3.1-pro` is priced with `service_tier=priority`
- **THEN** input, cache-read, cache-write, and output costs resolve to `26e-6`, `1.30e-6`, `26e-6`, and `136e-6` CNY per token respectively

#### Scenario: Compatibility spellings use canonical pricing

- **WHEN** pricing is resolved for `kimi3.1` or `moonshot/kimi-k3.1-pro`
- **THEN** the same standard and priority rates as `kimi-k3.1-pro` apply

#### Scenario: Legacy Kimi pricing does not capture K3.1 Pro

- **WHEN** both `kimi-k3.1-pro` and the legacy `kimi-k3` fallback are available
- **THEN** K3.1 Pro resolves only its exact entry and never inherits the legacy `kimi-k3` rates
