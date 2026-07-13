## ADDED Requirements

### Requirement: Tenant-Isolated Grok Cache Identity
The system SHALL derive Grok prompt-cache routing identity from a downstream API key, normalized upstream model, and request-specific conversation seed. The identity SHALL NOT expose the raw client seed or be shared across downstream API keys.

#### Scenario: Same tenant and conversation reuse cache identity
- **GIVEN** two Grok requests use the same downstream API key, normalized upstream model, and conversation seed
- **WHEN** the gateway builds their upstream requests
- **THEN** both requests use the same derived Grok cache identity

#### Scenario: Different tenants do not share cache identity
- **GIVEN** two Grok requests use different downstream API keys
- **AND** their raw conversation seed and model are otherwise identical
- **WHEN** the gateway derives their cache identities
- **THEN** the resulting identities are different

#### Scenario: Incomplete context disables cache identity
- **GIVEN** a Grok request has no valid downstream API key or no usable conversation seed
- **WHEN** the gateway builds the upstream request
- **THEN** it does not create a shared fallback cache identity
- **AND** it removes any untrusted Responses cache key before forwarding

### Requirement: Grok Cache Identity Propagation
The system SHALL apply the derived identity consistently to Grok cache-capable request bodies and headers across Responses, Messages, compatible Chat bridging, retry, failover, and WebSocket HTTP fallback paths.

#### Scenario: Responses request carries paired cache identity
- **GIVEN** an eligible Grok Responses request with a derived cache identity
- **WHEN** the upstream request is built
- **THEN** its `prompt_cache_key` equals its `X-Grok-Conv-Id` header

#### Scenario: Account failover preserves cache identity
- **GIVEN** a Grok request fails over from one eligible upstream account to another
- **WHEN** the request is retried for the same downstream tenant and conversation
- **THEN** the second upstream request uses the same cache identity as the first

#### Scenario: Cache identity remains separate from Codex session identity
- **GIVEN** a Grok request has a derived cache identity but no Codex `session_id` or `conversation_id`
- **WHEN** transport and sticky-session routing are resolved
- **THEN** the cache identity is not promoted into Codex session affinity or transport-connection identity

### Requirement: Free OAuth Cache-Capable Routing
The system SHALL add cache-capable native routing hints to eligible tool-free Grok OAuth requests without enabling tool execution or overriding explicit client tool intent.

#### Scenario: Tool-free Free OAuth request selects cache-capable route
- **GIVEN** an eligible Grok OAuth request has a cache identity
- **AND** the client specified no tools, functions, tool choice, or function call
- **WHEN** the Responses body is prepared
- **THEN** the gateway adds native `web_search` and `x_search` tools
- **AND** it sets `tool_choice` to `none`

#### Scenario: Explicit client tool intent is preserved
- **GIVEN** a Grok request explicitly contains tools, functions, tool choice, or function call intent
- **WHEN** the Responses body is prepared
- **THEN** the gateway does not inject Free OAuth native routing tools
- **AND** it preserves the existing tool-routing semantics

### Requirement: Cache-Capable Grok Chat Bridge
The system SHALL route only strictly compatible Grok Chat Completions requests through the cache-capable Responses endpoint and SHALL retain raw Chat Completions routing for incompatible request shapes.

#### Scenario: Compatible text Chat request uses Responses
- **GIVEN** a Grok 4.5 Chat Completions request contains only supported text-message and sampling fields
- **AND** a valid cache identity can be derived
- **WHEN** the request is forwarded
- **THEN** the gateway sends it to the Grok `/v1/responses` endpoint
- **AND** it converts the upstream response back to Chat Completions format

#### Scenario: Unsupported Chat shape stays raw
- **GIVEN** a Grok Chat Completions request contains stop sequences, structured message content, active tools, unsupported reasoning controls, or unknown fields
- **WHEN** routing eligibility is evaluated
- **THEN** the gateway sends the request through the existing raw `/v1/chat/completions` path
- **AND** it does not silently drop unsupported semantics

### Requirement: Grok Cached Usage Preservation
The system SHALL preserve upstream Grok cached-input token usage through synchronous and streaming Responses, Chat Completions, and Messages compatibility responses.

#### Scenario: Cached tokens survive response conversion
- **GIVEN** the Grok upstream reports cached input tokens
- **WHEN** the gateway converts the response to a downstream-compatible protocol
- **THEN** the resulting usage records and response usage expose the same cached-input token count

### Requirement: Non-Cacheable Grok Requests Remain Unchanged
The system SHALL keep compact requests, media generation, auxiliary probes, and requests without a valid cache identity on their established non-cacheable routes.

#### Scenario: Auxiliary image probe is not conversation-cached
- **GIVEN** the gateway sends an auxiliary Grok image-description probe
- **WHEN** the upstream request is built
- **THEN** it has no conversation cache identity

#### Scenario: Compact request is not augmented
- **GIVEN** a Grok `/responses/compact` request
- **WHEN** the upstream body and headers are prepared
- **THEN** the gateway does not inject cache identity or Free OAuth native routing tools
