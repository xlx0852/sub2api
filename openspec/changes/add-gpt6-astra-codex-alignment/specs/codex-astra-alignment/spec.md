## ADDED Requirements

### Requirement: Official Astra catalog metadata

The system SHALL preserve the official Codex Astra capability metadata, including base and maximum context windows, supported reasoning levels, Responses Lite, verbosity, tool mode, multi-agent version, and replay-related limits.

#### Scenario: Astra catalog entry is loaded

- **WHEN** the embedded catalog is loaded
- **THEN** `gpt-6-astra` exposes its official Codex metadata without changing unrelated model entries

### Requirement: Astra discovery fallback

The system SHALL provide a validated Astra Codex manifest entry when live OAuth model discovery is unavailable, while account entitlement checks remain authoritative for routing.

#### Scenario: OAuth discovery is unavailable

- **WHEN** the Codex models endpoint times out or returns an unusable response
- **THEN** the client receives the last-known-good or embedded Astra manifest and routing still rejects accounts without Astra entitlement

### Requirement: Astra reasoning and Responses Lite compatibility

The system SHALL preserve Astra's `low`, `medium`, `high`, `xhigh`, `max`, and `ultra` reasoning contract and SHALL forward Responses Lite metadata when Astra requests it.

#### Scenario: Astra request uses ultra

- **WHEN** a Codex client sends an Astra request with `reasoning.effort=ultra`
- **THEN** the gateway preserves the effort unless the exact upstream endpoint explicitly requires a documented downgrade

### Requirement: Long-lived Astra WebSocket continuity

The system SHALL rotate or reconnect an Astra upstream WebSocket before the provider's 60-minute connection limit when a safe turn boundary exists.

#### Scenario: Astra session reaches the rotation window

- **WHEN** an upstream Astra connection is older than the configured proactive rotation threshold between turns
- **THEN** the gateway establishes a replacement connection and continues the session using valid replay state
