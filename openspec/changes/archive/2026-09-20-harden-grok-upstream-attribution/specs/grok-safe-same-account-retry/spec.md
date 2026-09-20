## ADDED Requirements

### Requirement: Explicit Grok Retry Suppression
The gateway SHALL honor an upstream Grok `x-should-retry: false` response as an instruction not to retry, fail over, or penalize the selected account for that response.

#### Scenario: Explicit false suppresses all retry paths
- **GIVEN** a Grok Responses or Chat attempt returns a normally retryable or failover-eligible status
- **AND** the upstream response contains `x-should-retry: false`
- **WHEN** the gateway handles the response before downstream output
- **THEN** it does not retry the same account, select another account, or apply an account health penalty

#### Scenario: Explicit false preserves the upstream error
- **GIVEN** a Grok upstream response contains `x-should-retry: false`
- **WHEN** automatic retry and failover are suppressed
- **THEN** the gateway returns the existing endpoint-compatible representation of the original upstream error

#### Scenario: Other header values preserve existing policy
- **GIVEN** `x-should-retry` is absent, true, or not an exact boolean false value after whitespace trimming
- **WHEN** the gateway classifies the Grok failure
- **THEN** existing status-based same-account retry, failover, and penalty behavior remains unchanged
