## ADDED Requirements

### Requirement: Grok Unauthorized Credential Attribution
The gateway SHALL classify Grok OAuth 401 responses against the credential actually sent as `stale`, `current`, or `unknown` without recording credential material.

#### Scenario: Rotated credential is stale
- **GIVEN** a Grok request was sent with an OAuth Token whose non-secret fingerprint differs from the credential owner's latest Token
- **WHEN** the upstream returns 401
- **THEN** the gateway records `stale` attribution and does not apply an account health penalty for that response

#### Scenario: Current credential remains penalized
- **GIVEN** the Token actually sent matches the credential owner's latest OAuth Token
- **WHEN** the upstream returns 401
- **THEN** the gateway records `current` attribution and preserves the existing unauthorized-account penalty and failover policy

#### Scenario: Unknown attribution is conservative
- **GIVEN** the latest credential cannot be loaded or compared reliably
- **WHEN** the upstream returns 401
- **THEN** the gateway records `unknown` attribution and preserves the existing unauthorized-account penalty and failover policy

#### Scenario: Attribution is secret-safe
- **GIVEN** any Grok 401 attribution result
- **WHEN** logs, Ops metadata, or errors are emitted
- **THEN** they may contain credential versions and irreversible fingerprints but MUST NOT contain the Token, API key, or a reversible credential fragment

#### Scenario: Shadow account uses credential owner
- **GIVEN** the selected Grok account delegates authentication to another credential-owning account
- **WHEN** a 401 is attributed
- **THEN** the gateway compares the sent credential with the latest state of the credential owner
