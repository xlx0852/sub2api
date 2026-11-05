## ADDED Requirements

### Requirement: Kimi K3.1 Pro upstream aliases

The system SHALL normalize Kimi K3.1 Pro compatibility spellings to the official upstream model ID
`kimi-k3.1-pro` only for Kimi account traffic. Matching SHALL be case-insensitive after trimming the
requested model name. A trailing `[1m]` marker SHALL be removed, and a trailing parenthesized thinking
suffix SHALL be preserved.

#### Scenario: Public canonical ID is preserved

- **WHEN** a Kimi request uses `kimi-k3.1-pro`
- **THEN** the upstream request uses `kimi-k3.1-pro`

#### Scenario: Short compatibility ID is expanded

- **WHEN** a Kimi request uses `kimi3.1`
- **THEN** the upstream request uses `kimi-k3.1-pro`

#### Scenario: Provider-prefixed ID is unwrapped

- **WHEN** a Kimi request uses `moonshot/kimi-k3.1-pro`
- **THEN** the upstream request uses `kimi-k3.1-pro`

#### Scenario: Context marker and thinking suffix are handled

- **WHEN** a Kimi request uses a case variant of `kimi3.1[1m](high)`
- **THEN** the upstream request uses `kimi-k3.1-pro(high)`

#### Scenario: Existing Kimi aliases are unchanged

- **WHEN** a Kimi request uses an existing K3 or K2.7 compatibility name
- **THEN** its current upstream mapping remains unchanged
