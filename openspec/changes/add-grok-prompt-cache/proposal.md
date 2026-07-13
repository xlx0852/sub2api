# Change: Add Grok Prompt Cache Routing

## Why

Grok responses can report cached-token usage, but the gateway does not currently create a stable, tenant-isolated cache identity or route eligible Chat Completions traffic through the cache-capable Responses surface. As a result, cache hits depend on callers supplying compatible upstream fields and Free OAuth traffic does not reliably enter xAI's cacheable route.

## What Changes

- Derive a stable Grok prompt-cache identity from the downstream API key, upstream model, and explicit or content-derived conversation seed
- Replace caller-provided upstream cache identifiers with the tenant-isolated identity
- Apply the same identity to Grok Responses bodies and `X-Grok-Conv-Id` headers across HTTP, Messages, WebSocket, retry, and failover paths
- Add the Free OAuth native-tool routing hints only for tool-free requests without explicit tool intent
- Route strictly compatible Grok Chat Completions requests through `/v1/responses` and translate Responses output back to Chat Completions
- Keep unsupported Chat request shapes, media probes, compact requests, and non-Grok platforms on their existing paths
- Preserve cached-token usage through synchronous and streaming response conversion

## Impact

- Affected specs: `grok-prompt-cache`
- Affected code: Grok request builders, Responses and Chat routing, Messages compatibility, WebSocket HTTP bridge, failover scheduling context, account tests, and Grok cache usage tests
- No database migration, frontend change, or pricing-rule change
