## MODIFIED Requirements

### Requirement: Single account scheduler

The system SHALL provide one `AccountSelector` entry point for all platforms, composed of named hard candidate policies, explicit affinity decisions, a shared scoring and TopK selection core, bounded runtime feedback, and platform acquire/wait adapters. Platform-specific behavior MUST be expressed as policies or adapters rather than separate scheduler implementations.

Hard eligibility policies MUST run before scoring and MUST NOT be bypassed by priority, load, TTFT, error-rate, quota-headroom, affinity weights, or exploration. The scoring path SHALL be introduced through default-off shadow and scoped enforce rollout, and duplicate legacy scheduler bodies MUST remain until differential, canary, billing, fault, and rollback gates pass for every target platform.

#### Scenario: Scheduling behavior is compared before enforcement

- **WHEN** a platform is migrated toward the shared scoring core
- **THEN** the legacy and shared implementations receive the same immutable candidate input in shadow mode
- **AND** candidate-policy, affinity, score, TopK, selection, and wait-plan differences are recorded without shared evaluation producing live side effects

#### Scenario: A hard policy rejects a scored candidate

- **WHEN** quota cooldown, credential class, capability, RPM/window-cost, group, or another declared hard policy rejects an account
- **THEN** the shared scorer cannot select that account regardless of its score

#### Scenario: Scheduling behavior parity after unification

- **WHEN** all platform migration gates pass and the two legacy scheduler bodies are removed
- **THEN** one Engine with shared policies/scoring and explicit platform adapters serves all production selection paths
- **AND** sticky-session, capability, cooldown, quota, failover, wait-plan, mixed-platform, and billing invariants remain covered by long-term regression and replay tests
