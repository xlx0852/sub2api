# architecture-governance Specification Delta

## MODIFIED Requirements

### Requirement: Single account scheduler

The system SHALL provide one `AccountSelector` entry point for all platforms, composed of policy plugins (priority, load awareness, sticky sessions, capability filtering, quota pause and cooldown, mixed scheduling). Platform-specific behavior MUST be expressed as policies rather than separate scheduler implementations. When a selection yields no candidate, the scheduler MUST emit the per-candidate policy rejection trace so that availability gating is diagnosable without manual field-by-field inspection.

#### Scenario: Scheduling behavior parity after unification
- **WHEN** the two legacy schedulers are merged behind the unified interface
- **THEN** existing sticky-session, capability, and cooldown semantics are preserved for every platform as verified by regression tests

#### Scenario: Empty candidate set is diagnosable
- **WHEN** a selection returns "no available accounts"
- **THEN** the emitted evidence identifies which policy rejected each platform-level candidate, distinguishing a genuinely empty pool from model/capability/cooldown filtering
