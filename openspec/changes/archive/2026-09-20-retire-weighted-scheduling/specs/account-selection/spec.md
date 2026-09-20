## ADDED Requirements

### Requirement: One ordering algorithm for every provider
The system SHALL select all providers through the shared priority, current concurrent load and randomized equal-tier ordering. Historical score weights, TopK, rollout modes and OpenAI group scopes MUST NOT select an alternative production algorithm.

#### Scenario: A new group or an unconfigured platform receives traffic
- **WHEN** a request has eligible accounts in its allowed provider/group
- **THEN** all candidates use priority/load/equal-shuffle without opt-in settings

#### Scenario: No performance history exists
- **WHEN** two eligible candidates have equal priority and load but different observation histories
- **THEN** both may be selected first, independent of TTFT, quota headroom or cached tickets

### Requirement: Platform constraints remain authoritative
The system SHALL preserve provider/model/transport/group, quota/cooldown, channel, privacy, RPM/window-cost and session-count gates, atomically acquire and release slots, and retain bounded waiting and model-global limits.

#### Scenario: Ordinary preferred account is busy
- **WHEN** a valid sticky account cannot acquire a slot
- **THEN** other eligible accounts are attempted before waiting

#### Scenario: Compact is unsupported
- **WHEN** final account state explicitly reports unsupported compact capability
- **THEN** that account is not used for compact even if its priority and load are best

#### Scenario: Model routes or mixed pools apply
- **WHEN** an Anthropic model route or Gemini/Antigravity mixed pool is evaluated
- **THEN** ordering is shared but candidates cannot bypass existing route/mixed eligibility constraints

### Requirement: Protocol and credential continuity survive ordering retirement
The system SHALL preserve non-movable continuation affinity, tool pairing and request-local authentication-failure credential isolation. Simplification MUST NOT edit caller context or falsely treat an upstream failure as success.

#### Scenario: Continuation account is unavailable
- **WHEN** previous_response_id requires an unavailable original account and context is not safely replayable
- **THEN** return a continuation error rather than selecting an unrelated account

### Requirement: Retired scoring controls cannot mislead operators
The administrator UI SHALL explain the active simple algorithm instead of offering score weights, TopK or rollout controls. Legacy stored settings MAY remain for compatibility but MUST NOT change scheduling or be shown as an effective composite score.

#### Scenario: An old client writes scoring settings
- **WHEN** stored historical settings are updated
- **THEN** the active selector remains priority/load/equal-shuffle
