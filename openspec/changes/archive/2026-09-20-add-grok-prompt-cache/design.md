## Context

The current Grok integration can parse `cached_tokens`, and a caller-provided `prompt_cache_key` may survive the Responses sanitizer. It does not isolate that key per downstream tenant, does not pair it with `X-Grok-Conv-Id`, and sends Grok Chat Completions directly to `/v1/chat/completions`. Upstream v0.1.152 adds a complete cache identity and Chat-to-Responses bridge, but the local branch has additional Grok API-key routing, CLI header, media, Messages, and WebSocket behavior that must be preserved during integration.

## Goals / Non-Goals

- Goals:
  - Enable repeatable Grok prompt-cache hits without sharing cache identity between downstream API keys
  - Preserve one identity across retries and account failover for the same downstream request
  - Enable cache-capable Free OAuth routing without allowing injected search tools to execute
  - Preserve cached-token accounting in sync and stream responses
  - Keep current Grok API-key, media, CLI proxy, Messages, and WebSocket behavior intact
- Non-Goals:
  - Treat `prompt_cache_key` as a Codex thread or sticky-session identity
  - Cache media-generation or auxiliary image-description probes
  - Change Grok pricing, quota deduction, or database schema
  - Force incompatible Chat Completions request shapes through Responses

## Decisions

- Decision: derive a versioned identity from downstream API key ID, normalized upstream model, and a request seed
  - Explicit session, conversation, Grok conversation header, body cache key, or route parameter may provide the seed
  - Content-derived seed is used only when no explicit seed exists
  - A missing downstream API key, model, or seed disables cache identity instead of creating a shared fallback

- Decision: replace rather than trust caller-provided upstream cache identity
  - This prevents two tenants using the same OAuth account from colliding in xAI's server-side cache
  - The derived identity is written to Responses `prompt_cache_key` and `X-Grok-Conv-Id`

- Decision: keep Grok cache identity separate from Codex session affinity
  - The value is an upstream Grok cache-routing hint only
  - It is not promoted to `session_id`, Codex sticky routing, or transport-connection identity

- Decision: inject Free OAuth native tools only when the client expressed no tool intent
  - `web_search` and `x_search` select the cache-capable route
  - `tool_choice=none` prevents either injected tool from executing
  - Any explicit `tools`, `functions`, `tool_choice`, or `function_call` keeps client semantics and disables the augmentation

- Decision: bridge only strictly compatible Chat Completions shapes
  - Simple text messages for the cache-capable Grok model may use `/v1/responses`
  - Stop sequences, explicit reasoning controls, structured content, active tools, conflicting token limits, and unknown fields stay on raw Chat Completions

## Risks / Trade-offs

- Risk: synthetic routing tools could change model behavior
  - Mitigation: inject only for otherwise tool-free Free OAuth requests and force `tool_choice=none`
- Risk: Chat-to-Responses conversion could drop unsupported Chat semantics
  - Mitigation: use a strict allowlist and fall back to raw Chat Completions on any unsupported field or message shape
- Risk: cache identities could leak or collide across tenants
  - Mitigation: hash a versioned seed containing the downstream API key ID and normalized model, and replace raw client identities before upstream transmission
- Risk: integration can regress current custom Grok routing
  - Mitigation: preserve existing API-key/CLI/media/WS branches and validate actual upstream URL, headers, body, cache usage, retry, and failover behavior

## Migration Plan

1. Integrate the two upstream cache commits into the current Grok routing implementation
2. Resolve conflicts in favor of current API-key, CLI proxy, media, Messages, and WebSocket behavior
3. Add cache identity and Chat bridge regression tests before changing production call sites
4. Run targeted Grok, Messages, WebSocket, handler, and server-entry tests
5. Deploy with the existing blue-green workflow and retain the previous production container for rollback
