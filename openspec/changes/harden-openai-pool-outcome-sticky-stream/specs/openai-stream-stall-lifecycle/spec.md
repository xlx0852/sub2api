## ADDED Requirements

### Requirement: Stream Keepalive And Upstream Progress Use Separate Clocks

For OpenAI Responses streaming (SSE and equivalent bridged streams), the system SHALL maintain two distinct timing concepts:

- client keepalive arming, which may emit non-terminal keepalive/heartbeat frames to prevent client or proxy idle disconnects
- upstream stall detection, which tracks silence since the last real upstream progress event

Client keepalive frames MUST NOT reset the upstream stall clock. Real upstream bytes or parsed semantic events MUST reset the upstream stall clock.

#### Scenario: Keepalive alone does not count as upstream progress

- **GIVEN** a Responses SSE request has started
- **AND** the gateway is emitting keepalive/heartbeat frames
- **AND** no upstream progress event has arrived for the stall timeout budget
- **WHEN** the stall clock is evaluated
- **THEN** the stream is treated as stalled despite keepalives

#### Scenario: Semantic upstream event resets stall clock

- **GIVEN** a stream is open and the stall clock is counting
- **WHEN** a real upstream progress event is received and forwarded or accepted by the bridge
- **THEN** the upstream stall clock resets

### Requirement: Upstream Stall Terminates As Incomplete Not Completed

When the upstream stall budget elapses, the system SHALL:

1. finalize or close any open non-terminal output items as required for protocol validity
2. emit a terminal incomplete outcome to the client with an explicit reason such as `upstream_stall_timeout`
3. cancel the upstream request
4. attempt normal usage settlement for any known partial usage

The system MUST NOT mark a stalled truncated stream as a successful `completed` response.

#### Scenario: Mid-stream silence ends with response.incomplete

- **GIVEN** a Responses stream already delivered some progress
- **AND** no further upstream progress arrives within the configured stall timeout
- **WHEN** the stall watchdog fires
- **THEN** the client receives a terminal `response.incomplete` (or documented equivalent incomplete status)
- **AND** the incomplete reason identifies upstream stall timeout
- **AND** the upstream request is cancelled
- **AND** no later `response.completed` success terminal is emitted for that attempt

#### Scenario: Stall path settles usage without double terminal events

- **GIVEN** a stream ends via upstream stall handling
- **WHEN** the client disconnects at the same time or usage finalization runs
- **THEN** the gateway writes at most one terminal stream outcome
- **AND** usage settlement follows the existing incomplete/partial path without requiring a fake completed status

### Requirement: Stall Timeout Is Configurable And Default-Enabled For OpenAI Responses

The system SHALL expose a configuration knobs for stream stall timeout and a kill switch. OpenAI Responses streaming SHOULD default to a positive stall timeout suitable for production idle protection (recommended default 300 seconds) rather than disabled-by-default behavior.

First-output timeout, when configured, covers time-to-first-upstream-progress and MUST NOT disable mid-stream stall detection merely by being zero/unset.

#### Scenario: Default stall protection applies without first-output timeout

- **GIVEN** first-output timeout is unset or zero
- **AND** stream stall timeout uses the default positive value
- **WHEN** a Responses stream goes silent after start for longer than the stall timeout
- **THEN** stall incomplete handling still runs

#### Scenario: Operators can disable stall enforcement

- **GIVEN** the stall kill switch/timeout is explicitly disabled by configuration
- **WHEN** a long-running stream emits only keepalives for an extended period
- **THEN** the gateway does not force an incomplete terminal solely due to the stall watchdog

### Requirement: Compact Soft-Timeout Semantics Remain Distinct

General stream stall handling MUST NOT silently replace compact-specific soft-timeout retry behavior. Compact paths MAY emit keepalives and MAY perform at most one protocol-safe account retry under their own change/spec. General stall incomplete handling applies to ordinary Responses streams and to compact streams only where those compact specs do not define a more specific pre-terminal retry contract.

#### Scenario: Ordinary responses stream does not inherit compact retry-on-soft-timeout

- **GIVEN** an ordinary non-compact Responses stream stalls
- **WHEN** the stall budget elapses before any terminal event
- **THEN** the gateway terminates with incomplete per this capability
- **AND** it does not automatically replay the full user request on another account unless a separate non-compact retry policy explicitly allows it
