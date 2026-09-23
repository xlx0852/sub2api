## MODIFIED Requirements

### Requirement: Private External Provider Bridges

The system SHALL run Cursor and Devin provider bridges as privately reachable,
health-checked services and SHALL route migrated Sub2API accounts through their
OpenAI-compatible HTTP endpoints. The Devin bridge SHALL be the pinned
CLIProxyAPI release, exposing both `/v1/chat/completions` and `/v1/responses`
for the Devin model catalog behind a client API key, and SHALL NOT publish a
public listener. The bridge SHALL publish the model ids the gateway routes on,
including the standard ids for the upstream GPT family, instead of only the
upstream's own namespaced ids.

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

#### Scenario: Upstream-specific ids are translated to standard ids

- **WHEN** the bridge exposes an upstream model whose id is not one our clients
  use (for example `devin/gpt-5-5`)
- **THEN** it SHALL accept our standard id for that model (`gpt-5.5`)
- **AND** it SHALL answer with that standard id so the client sees the model it
  asked for, on streaming and non-streaming responses alike

## ADDED Requirements

### Requirement: Devin GPT capacity is an overflow provider for standard ids

The Devin account SHALL be registered as a provider for the standard ids of the GPT family its upstream
can serve, and SHALL sit in the lowest scheduling priority tier so it is used as overflow rather than as
a peer, leaving existing provider traffic distribution unchanged.

#### Scenario: Standard id is served by the existing accounts first

- **GIVEN** the Devin account is mapped for `gpt-5.5` and holds the lowest priority tier
- **WHEN** a client requests `gpt-5.5` and a higher-tier account is able to take it
- **THEN** the request SHALL be served by that account
- **AND** the Devin account SHALL only be selected when no higher-tier account can take it

#### Scenario: Devin-served standard ids are billed normally

- **WHEN** Devin serves a request for a standard id
- **THEN** the usage record SHALL keep the standard model id and be priced from the existing price entry
- **AND** the record's account SHALL identify Devin as the provider

#### Scenario: Wired ids stay unreachable until a group exposes them

- **GIVEN** an id is mapped on the Devin account but absent from every group's model list
- **WHEN** a client requests that id
- **THEN** the gateway SHALL reject the model as it does today
- **AND** enabling it SHALL require only adding the id to the group's model list
