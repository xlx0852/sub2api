## ADDED Requirements

### Requirement: Grok Request-Class Upstream Routing
The system SHALL resolve Grok upstream endpoints from the account authentication kind, the explicit `using_api` credential, and the request class.

#### Scenario: OAuth text HTTP defaults to CLI proxy
- **GIVEN** a Grok OAuth account without an explicit `using_api` credential
- **WHEN** a Responses or Chat Completions HTTP request is forwarded
- **THEN** the system uses `https://cli-chat-proxy.grok.com/v1`

#### Scenario: Explicit official API override
- **GIVEN** a Grok account with `using_api=true`
- **WHEN** a text HTTP request is forwarded
- **THEN** the system uses the account's official API base URL

#### Scenario: Custom gateway remains explicit
- **GIVEN** a Grok account with a non-default custom base URL
- **WHEN** a text HTTP request is forwarded
- **THEN** the system preserves that custom base URL

#### Scenario: Media and WebSocket remain official
- **GIVEN** a Grok OAuth account whose text HTTP traffic uses the CLI proxy
- **WHEN** an image, video, or native WebSocket request is forwarded
- **THEN** the system uses the official xAI API or an explicit non-CLI custom gateway
- **AND** it does not use the HTTP-only CLI proxy

### Requirement: Grok CLI HTTP Identity
The system SHALL attach Grok CLI identity headers only when text HTTP traffic targets the official CLI chat proxy.

#### Scenario: CLI proxy receives identity headers
- **GIVEN** a Grok text HTTP request resolved to the official CLI proxy
- **WHEN** the upstream request is built
- **THEN** it includes the Grok CLI token-auth and client-version headers

#### Scenario: Official API and custom gateways do not receive CLI identity headers
- **GIVEN** a Grok request resolved to the official API or a custom gateway
- **WHEN** the upstream request is built
- **THEN** the system does not inject Grok CLI identity headers
