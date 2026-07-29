## ADDED Requirements

### Requirement: Provider Refresh Cooldown
The system SHALL honor an OAuth token provider's explicit retry time across application instances and SHALL avoid refreshing the same credential before that time.

#### Scenario: Claude refresh rate limited
- **WHEN** the Claude token endpoint returns HTTP 429 with `Retry-After` or `Retry-After-Ms`
- **THEN** the account refresh is deferred until the bounded retry time and immediate fixed-delay retries are suppressed

#### Scenario: Cooldown expires
- **WHEN** the stored refresh cooldown time passes
- **THEN** a worker may attempt token refresh through the existing distributed refresh coordinator

#### Scenario: Non-rate-limit refresh failure
- **WHEN** a refresh fails without an explicit provider retry time
- **THEN** the existing retry classification and bounded exponential backoff remain in effect
