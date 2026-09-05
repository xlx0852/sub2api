## MODIFIED Requirements

### Requirement: Provider forwarding lifecycle
Provider forwarding SHALL expose explicit lifecycle boundaries for parse, admission, rewrite, transport, relay, and settlement. A select boundary SHALL exist only when it performs account selection or another observable policy decision; a no-op select stage MUST NOT be retained solely as an extension point.

#### Scenario: No-op select stage is removed
- **WHEN** account selection has completed before provider forwarding and the pipeline select stage performs no policy decision
- **THEN** the pipeline omits that stage while preserving admission, rewrite, transport, relay, settlement, response, usage, and lease behavior

#### Scenario: Selection policy is present
- **WHEN** a provider requires an observable account selection policy during forwarding
- **THEN** the pipeline retains an explicit select stage and its behavior is covered by regression tests
