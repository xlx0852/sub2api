## Context

xAI exposes separate upstream surfaces. The CLI chat proxy serves OAuth text HTTP traffic, while the official API serves API-key traffic, media endpoints, and native WebSocket upgrades. A persisted `base_url` alone cannot represent this request-type-dependent routing.

## Goals / Non-Goals

- Goals:
  - Match CPA endpoint selection for Grok HTTP chat
  - Keep existing custom gateway support
  - Keep media and native WebSocket requests away from the HTTP-only CLI proxy
  - Allow an account credential to explicitly select the official API
- Non-Goals:
  - Add Grok API-key forwarding where the existing service only supports OAuth
  - Change Grok model mapping or billing behavior
  - Change non-Grok account routing

## Decisions

- Decision: resolve endpoint by request class instead of mutating persisted `base_url`
  - Text HTTP uses an account-level chat resolver
  - Media and WebSocket use an official-capable resolver
- Decision: `credentials.using_api` accepts JSON boolean or boolean string
  - Explicit values override defaults
  - OAuth defaults to CLI chat proxy; non-OAuth defaults to official API
- Decision: explicit non-default custom base URLs are preserved
  - This keeps test gateways and private compatible upstreams functional
- Decision: CLI identity headers are attached only for the exact official CLI proxy host
  - Custom gateways do not receive Grok CLI fingerprint headers implicitly

## Risks / Trade-offs

- OAuth text behavior changes from official API to CLI proxy by default
  - Mitigation: `using_api=true` restores the official API path per account
- Legacy accounts explicitly storing the CLI proxy cannot use that URL for media
  - Mitigation: media resolution rewrites only the known CLI proxy URL to the official API

## Migration Plan

1. Add request-class-specific Grok base URL resolvers
2. Route Responses, Chat Completions, and account tests through the text resolver
3. Route media and native WebSocket URL construction through the official-capable resolver
4. Add CLI identity headers on official CLI proxy requests
5. Run targeted service tests before deployment
