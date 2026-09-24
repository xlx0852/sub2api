## ADDED Requirements

### Requirement: Request timing detail segments

The account usage request-timing detail SHALL expose segmentation for the inbound request body, gateway internal stages, upstream transport stages, and the request result, and SHALL mark any segment that was not collected instead of omitting the row.

#### Scenario: Gateway records a streamed Responses request

- **WHEN** a client sends a streamed request through the OpenAI gateway
- **THEN** the stored usage detail SHALL include the inbound body read wait, receive span, byte size, and completeness
- **AND** it SHALL include API key authentication, user concurrency wait, account selection, account concurrency admission, handler body read, and handler total durations
- **AND** it SHALL include upstream connection acquisition, request write, first byte wait, response header readiness, first token, and stream transfer durations for each upstream attempt
- **AND** it SHALL include the attempt count, connection reuse flag, request body completeness, response stream completeness, and the outcome

#### Scenario: Segment was not collected

- **WHEN** a record predates this capture or came from a path without instrumentation
- **THEN** the detail SHALL keep the row visible with an explicit not-collected state
- **AND** it SHALL NOT substitute zero for a missing measurement

#### Scenario: Upstream request fails before first byte

- **WHEN** an upstream attempt fails before the first response byte
- **THEN** the detail SHALL still record that attempt's connection and write stages
- **AND** the failed attempt SHALL be counted in the attempt total while the retry/failover duration remains separately visible
