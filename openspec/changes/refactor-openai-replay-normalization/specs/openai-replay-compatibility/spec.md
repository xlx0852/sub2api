## ADDED Requirements

### Requirement: Authoritative replay assembly
The system SHALL prefer terminal Responses output snapshots over earlier partial item events when constructing replay input.

#### Scenario: Completed output replaces a partial item
- **WHEN** `response.output_item.done` contains an incomplete tool call and `response.completed` later contains the same complete tool call
- **THEN** the replay sequence contains the complete terminal item in terminal output order

#### Scenario: Partial event arrives after completed
- **WHEN** a duplicate partial event arrives after the terminal snapshot
- **THEN** it MUST NOT replace or duplicate the terminal item

### Requirement: Replay ownership boundary
The system SHALL apply tool-item compatibility conversion only to server-collected replay items unless an upstream explicitly rejects an indexed client item.

#### Scenario: Client input is already valid
- **WHEN** a client submits a valid Responses input item
- **THEN** replay preparation MUST preserve that client item without type or id rewriting

#### Scenario: Client call id is opaque
- **WHEN** Codex emits a tool call or output with `call_id=call_X`
- **THEN** the initial upstream request and every client-visible response MUST preserve `call_X` exactly; item `id` prefix rules MUST NOT be applied to `call_id`

#### Scenario: Server replay contains a private tool item
- **WHEN** a server-collected replay item uses a Codex-private tool type
- **THEN** the system converts it to a valid standard Responses item while preserving its `call_id`

### Requirement: Indexed compatibility repair
The system SHALL repair only the indexed item named by an explicit upstream unknown, unsupported, missing, or invalid field rejection and SHALL retry at most once for the resulting body.

#### Scenario: Upstream rejects private arguments on one item
- **WHEN** upstream explicitly rejects `input[N].arguments`
- **THEN** only item N and its call-linked semantics are normalized before a single retry

#### Scenario: Upstream rejects a private item id
- **WHEN** upstream explicitly rejects `input[N].id`
- **THEN** the rejected id is removed without changing `call_id` before a single retry

### Requirement: Replay structural validation
The system SHALL NOT inject incomplete tool calls or orphan tool outputs into replay input.

#### Scenario: Tool call arguments remain incomplete at terminal time
- **WHEN** no complete terminal tool call is available
- **THEN** the incomplete call and its linked output are excluded instead of fabricating empty arguments

#### Scenario: Mixed replay pair crosses a dialect boundary
- **WHEN** a server-collected `function_call` is merged with a client-supplied private output carrying the same `call_id`
- **THEN** the output is aligned to `function_call_output` while the call and `call_id` remain unchanged

#### Scenario: Client echoes a call item id as the output call id
- **WHEN** a client output uses the unique call item `id` instead of that item's required `call_id`
- **THEN** the replay adapter resolves the item-id alias and restores the call's real `call_id`

#### Scenario: Output has no real call context
- **WHEN** an output `call_id` has no matching call item in the self-contained replay sequence
- **THEN** the system MUST NOT fabricate a call or retry the orphan output as a valid pair

### Requirement: Explicit call-pair compatibility repair
The system SHALL allow at most one compatibility retry when an upstream explicitly rejects a mixed call/output pair by `call_id`.

#### Scenario: Upstream reports no matching custom tool call
- **WHEN** upstream returns `No tool call found for custom tool call output with call_id X` and input contains a real standard call plus private output for X
- **THEN** only items with `call_id X` are aligned before one retry

#### Scenario: Compatibility layer canonicalizes call prefix
- **WHEN** upstream rejects `fc_X` while the self-contained input has exactly one complete call/output pair using `call_X`
- **THEN** one outbound retry MAY translate both sides of that unique pair to `fc_X` without mutating client-visible history

#### Scenario: Prefix alias is ambiguous
- **WHEN** more than one call could match the rejected `call_`/`fc_` suffix
- **THEN** the system MUST NOT retry or guess the association

#### Scenario: Rejected call id belongs to a true orphan
- **WHEN** the rejected `call_id` has an output but no matching call item
- **THEN** the compatibility layer MUST NOT retry or manufacture missing context
