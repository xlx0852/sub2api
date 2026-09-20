## ADDED Requirements

### Requirement: Server-Derived Grok Request Identity
The gateway SHALL attach server-derived request, session, attempt, agent, conversation, and model identity to Grok HTTP inference requests without trusting client-supplied Grok identity headers.

#### Scenario: Identity is consistent across protocols
- **GIVEN** equivalent Grok requests use Responses, native Chat Completions, or Chat-via-Responses
- **WHEN** the gateway builds the upstream request
- **THEN** each path emits the same server-controlled identity contract appropriate to that request

#### Scenario: Attempts are monotonically identified
- **GIVEN** one downstream request performs a same-account retry or account failover
- **WHEN** each real upstream attempt begins
- **THEN** request and session identity remain stable while the attempt identity increases monotonically

#### Scenario: Tenant identities are isolated
- **GIVEN** two API keys submit otherwise identical sessions and models
- **WHEN** their Grok upstream identities are generated
- **THEN** their session and conversation identities differ and reveal neither API key nor raw client session values

#### Scenario: Client headers cannot override identity
- **GIVEN** a client supplies any `x-grok-*` identity header
- **WHEN** the gateway applies forwarded headers and account overrides
- **THEN** the final inference request contains only the gateway-derived identity values
