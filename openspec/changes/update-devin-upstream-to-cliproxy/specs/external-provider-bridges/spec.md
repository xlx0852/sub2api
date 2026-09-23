## MODIFIED Requirements

### Requirement: Private External Provider Bridges

The system SHALL run Cursor and Devin provider bridges as privately reachable,
health-checked services and SHALL route migrated Sub2API accounts through their
OpenAI-compatible HTTP endpoints. The Devin bridge SHALL be the pinned
CLIProxyAPI release, exposing both `/v1/chat/completions` and `/v1/responses`
for the Devin model catalog behind a client API key, and SHALL NOT publish a
public listener.

#### Scenario: Bridge cutover

- **WHEN** a Cursor or Devin sidecar passes health, model, and authenticated
  forwarding probes
- **THEN** the corresponding Sub2API account can be switched to the sidecar
  without changing the client-facing gateway endpoint

#### Scenario: Bridge failure

- **WHEN** a sidecar is unhealthy or rejects its credential
- **THEN** Sub2API reports the upstream failure and the previous native release
  and account snapshot remain available for rollback

#### Scenario: Devin sidecar serves both OpenAI-compatible endpoints

- **WHEN** Sub2API forwards a Devin request with the account's configured model
- **THEN** the sidecar SHALL answer non-streaming and streaming chat completions
  and responses requests for that model
- **AND** the sidecar SHALL reject requests that do not carry its client API key

#### Scenario: Devin cutover keeps the account contract

- **WHEN** the Devin account is repointed at a replacement sidecar
- **THEN** only the upstream base URL and the client API key SHALL change
- **AND** the account's model, model mapping, platform, bridge kind and the
  client-facing gateway endpoint SHALL remain as they were

## ADDED Requirements

### Requirement: Devin sidecar credential lifecycle

The Devin sidecar SHALL hold Devin credentials as managed auth records rather
than a value embedded in its configuration, so credentials can be rotated,
refreshed, and multiplied without editing the sidecar configuration.

#### Scenario: Credential import without a browser

- **WHEN** an operator provides an existing Devin session token
- **THEN** the sidecar SHALL create a credential record for it through its
  headless login flow
- **AND** the token SHALL NOT appear in sidecar configuration, logs, or the
  deploy record

#### Scenario: Credential health is observable

- **WHEN** the sidecar refreshes a Devin credential
- **THEN** the credential record SHALL carry the observed profile, plan and
  quota signals
- **AND** a rejected or expired credential SHALL surface as an explicit upstream
  failure to Sub2API instead of an anonymous transport error

#### Scenario: Multiple Devin credentials

- **WHEN** more than one Devin credential record exists
- **THEN** the sidecar SHALL expose the same model catalog and select among the
  records without any Sub2API account change
