## ADDED Requirements

### Requirement: Removed quota auto-pause controls have no enforcement path
The system SHALL remove the OpenAI 5h and 7d quota auto-pause controls and enforcement path. Historical per-account metadata MAY remain stored but MUST NOT affect account selection, sticky routing, or schedulability. Expiry auto-pause, provider cooldowns, and Grok quota behavior SHALL remain independent.

#### Scenario: OpenAI account exceeds historical 5h or 7d threshold
- **WHEN** an OpenAI account contains historical `auto_pause_5h_threshold` or `auto_pause_7d_threshold` metadata and its usage exceeds that value
- **THEN** OpenAI scheduling does not skip the account because of that metadata

#### Scenario: Admin opens Ops settings
- **WHEN** an administrator views advanced Ops settings
- **THEN** no 5h/7d OpenAI quota auto-pause controls or validation fields are rendered or accepted

#### Scenario: Expired account is evaluated
- **WHEN** an account reaches its configured expiration and `auto_pause_on_expired` is enabled
- **THEN** expiry handling continues unchanged
