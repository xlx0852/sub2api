# account-selection Delta

## MODIFIED Requirements

### Requirement: Platform constraints remain authoritative
The system SHALL preserve provider/model/transport/group, quota/cooldown, channel, privacy and model-global gates, atomically acquire and release slots, and retain bounded waiting limits. Per-account RPM, 5h window-cost and session-count gates that only applied to retired Anthropic OAuth/SetupToken supply MUST NOT be present.

#### Scenario: Ordinary preferred account is busy
- **WHEN** a valid sticky account cannot acquire a slot
- **THEN** other eligible accounts are attempted before waiting

#### Scenario: Compact is unsupported
- **WHEN** final account state explicitly reports unsupported compact capability
- **THEN** that account is not used for compact even if its priority and load are best

#### Scenario: Model routes or mixed pools apply
- **WHEN** an Anthropic model route or Gemini/Antigravity mixed pool is evaluated
- **THEN** ordering is shared but candidates cannot bypass existing route/mixed eligibility constraints

#### Scenario: API-key account quota limit applies
- **WHEN** an API-key account has an exceeded `quota_*` limit
- **THEN** that account remains excluded from scheduling
