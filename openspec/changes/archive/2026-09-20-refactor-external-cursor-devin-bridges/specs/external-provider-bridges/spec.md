## ADDED Requirements

### Requirement: Private External Provider Bridges

The system SHALL run Cursor and Devin provider bridges as privately reachable,
health-checked services and SHALL route migrated Sub2API accounts through their
OpenAI-compatible HTTP endpoints.

#### Scenario: Bridge cutover

- **WHEN** a Cursor or Devin sidecar passes health, model, and authenticated
  forwarding probes
- **THEN** the corresponding Sub2API account can be switched to the sidecar
  without changing the client-facing gateway endpoint

#### Scenario: Bridge failure

- **WHEN** a sidecar is unhealthy or rejects its credential
- **THEN** Sub2API reports the upstream failure and the previous native release
  and account snapshot remain available for rollback

### Requirement: External Provider Usage Projection

The system SHALL query supported sidecar usage endpoints and project available
provider credits, plan state, or usage windows into the existing account usage
surface, while returning an explicit unknown/degraded state when the sidecar
does not provide a stable field.

#### Scenario: Cursor usage query

- **WHEN** a migrated Cursor account requests a quota refresh
- **THEN** Sub2API calls the private Cursor bridge account endpoint and maps
  returned Sand/API usage fields without exposing the bearer token

#### Scenario: Devin usage query

- **WHEN** a migrated Devin account requests a quota refresh
- **THEN** Sub2API queries the configured Devin bridge status endpoint and
  reports available plan/credit fields or a visible unknown state
