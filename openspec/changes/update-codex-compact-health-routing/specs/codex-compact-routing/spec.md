## ADDED Requirements

### Requirement: Compact Requests Are Classified By Inbound Shape

The system SHALL classify Codex compact traffic into distinct modes after request inspection:

- `legacy_path` when the request targets `.../responses/compact`
- `body_signal_v2` when the request targets bare `.../responses` and `input` contains an item with `type=compaction_trigger`

Both modes MAY share compact scheduling and metrics classification, but SHALL NOT share a single forced wire format.

#### Scenario: Body-signal v2 is classified without path rewrite as the client contract

- **GIVEN** a Codex official client sends `POST /v1/responses` with `stream=true` and an `input` item `{type:"compaction_trigger"}`
- **WHEN** the gateway accepts the request
- **THEN** the request is marked compact mode `body_signal_v2`
- **AND** the client-facing path contract remains `/responses` streaming Responses SSE
- **AND** ordinary `/responses` requests without `compaction_trigger` keep their existing non-compact classification

#### Scenario: Path-based legacy compact stays JSON

- **GIVEN** a client sends `POST /v1/responses/compact`
- **WHEN** the gateway handles the request
- **THEN** the request is marked compact mode `legacy_path`
- **AND** the downstream response contract remains unary JSON compact output

### Requirement: Body-Signal V2 Preserves Codex SSE Terminal Contract

For `body_signal_v2`, the system SHALL provide a downstream Responses SSE stream that eventually yields:

1. exactly one successful compaction output item via `response.output_item.done` with `item.type=compaction`, and
2. a terminal `response.completed` event

unless the request fails, in which case the stream SHALL NOT silently end without an error outcome.

#### Scenario: Native upstream v2 stream is passed through

- **GIVEN** a `body_signal_v2` request
- **AND** the selected upstream supports remote compaction v2 over `/responses` streaming
- **WHEN** the gateway forwards the request
- **THEN** the gateway preserves streaming `/responses` semantics for that attempt
- **AND** downstream still surfaces a compaction output item and `response.completed`

#### Scenario: Legacy upstream JSON is bridged to v2 SSE

- **GIVEN** a `body_signal_v2` request
- **AND** the selected upstream can only complete compact via legacy `/responses/compact` JSON
- **WHEN** the upstream compact JSON succeeds
- **THEN** the gateway writes downstream SSE events including `response.output_item.done` with one `type=compaction` item
- **AND** then writes `response.completed`
- **AND** the Codex client can finish remote compaction v2 without `stream closed before response.completed`

#### Scenario: Bridge keeps the stream alive while upstream is slow

- **GIVEN** a `body_signal_v2` request using the legacy-upstream SSE bridge
- **AND** upstream compact takes longer than intermediate idle thresholds
- **WHEN** the gateway is still waiting for upstream JSON
- **THEN** the gateway emits compact-only SSE keepalives on the downstream stream
- **AND** those keepalives are not terminal events
- **AND** ordinary non-compact `/responses` behavior is unchanged

### Requirement: Legacy Path Compact Remains Non-Stream JSON

The system SHALL continue to treat explicit `.../responses/compact` requests as unary JSON compact requests and SHALL NOT require SSE terminal events for that mode.

#### Scenario: Explicit compact path does not force SSE bridge

- **GIVEN** a client calls `POST /v1/responses/compact` without relying on body-signal v2
- **WHEN** upstream returns compact JSON
- **THEN** the gateway returns JSON compact output
- **AND** it does not rewrite the response into Responses SSE solely because compact health routing is enabled

### Requirement: Compact Account Health Affects Compact Scheduling Only

The system SHALL temporarily lower compact scheduling priority for accounts with recent compact soft timeouts, client disconnects, or high compact latency without changing their ordinary sync, streaming, or WS scheduling priority.

#### Scenario: Slow compact account is avoided for later compact request

- **GIVEN** two eligible OpenAI accounts can serve Codex compact
- **AND** one account recently exceeded the compact soft-timeout threshold
- **WHEN** a later compact request is scheduled
- **THEN** the account with the recent compact penalty is treated as lower priority
- **AND** the same penalty does not affect non-compact requests

### Requirement: Compact Soft Timeout Can Retry Before Terminal Commit

The system SHALL allow at most one compact retry on another eligible account when the first compact upstream request exceeds the configured soft timeout and no irreversible downstream terminal compact result has been committed.

For `body_signal_v2`, keepalives alone do not count as terminal commit; `response.output_item.done` / `response.completed` do.

#### Scenario: First compact account is too slow before terminal events

- **GIVEN** a compact request is sent to an upstream account
- **AND** the upstream request exceeds the compact soft-timeout threshold
- **AND** the gateway has not yet written a terminal compact result to the client
- **WHEN** another eligible compact account is available
- **THEN** the gateway cancels the slow upstream request and retries compact once
- **AND** the retry count and both account choices are recorded

### Requirement: Compact Outcome Includes Mode, Bridge, And Delivery State

The system SHALL record compact mode, whether SSE bridge was used, whether a compact response was delivered to the client, whether the client context was canceled, and whether response writing failed.

#### Scenario: Upstream succeeds after client disconnect

- **GIVEN** a compact upstream request returns successfully
- **AND** the downstream client has already disconnected
- **WHEN** the gateway records the compact outcome
- **THEN** the outcome includes upstream success and client-disconnected state separately
- **AND** includes compact mode and bridge-used indicators when applicable

### Requirement: WS HTTP Bridge Replay Is Distinguishable From Compact

The system SHALL keep WS v2 HTTP bridge/replay usage distinguishable from compact usage in operator-facing usage records without changing the compact request type semantics.

#### Scenario: Large WS payload uses HTTP bridge

- **GIVEN** a Codex WS v2 request is forwarded through the HTTP bridge path
- **WHEN** the gateway records usage for that turn
- **THEN** the usage row remains `request_type=ws_v2`
- **AND** includes WS bridge payload metrics
- **AND** the admin usage UI labels the row as WS bridge/replay instead of compact
