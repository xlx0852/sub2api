## Context

Kimi Coding exposes an OAuth 2.0 Device Authorization Grant rather than the authorization-code callback used by the existing OpenAI and Grok account flows. Its OAuth accounts call the Kimi Coding API with a stable device identity and support both OpenAI Chat Completions and Anthropic Messages-compatible traffic.

Sub2API already has generic Kimi pricing and thinking compatibility, JSON credential storage, account proxies, and a distributed OAuth refresh coordinator. The implementation should reuse those foundations without treating Kimi as OpenAI Responses or leaking device-flow credentials to the browser.

## Goals / Non-Goals

- Goals:
  - Authenticate and create Kimi OAuth accounts through a device code
  - Keep Kimi credentials refreshed across multiple Sub2API instances
  - Route OpenAI-compatible and Anthropic-compatible requests to Kimi Coding
  - Preserve the account proxy and stable device identity for login, refresh, and inference
  - Keep existing OpenAI, Grok, and generic API-key behavior unchanged
- Non-Goals:
  - Discover an undocumented Kimi subscription quota endpoint
  - Add a Kimi API-key account workflow beyond the existing generic compatible-upstream support
  - Reuse CLIProxyAPI-specific branding in platform-identifying headers

## Decisions

- Decision: represent Kimi as `platform=kimi`, `type=oauth`
  - Platform and credential fields are JSON/string backed, so no database schema migration is required.
- Decision: use a Redis-backed device session and client-driven status polling
  - This works across instances and avoids a long-lived goroutine tied to the process that handled the start request.
- Decision: use an atomic polling lease and cancellation-safe versioned update
  - Concurrent status calls cannot duplicate token polls, and an in-flight poll cannot recreate a cancelled session.
- Decision: return a short-lived login ticket after authorization
  - The refresh token remains server-side; the final account create request consumes the ticket.
- Decision: reuse `OAuthRefreshAPI`
  - Kimi receives the same local mutex, distributed lock, database reread, credential versioning, and cache invalidation guarantees as other OAuth platforms.
- Decision: route Kimi Responses and Messages input through Chat Completions conversion
  - Kimi Coding's subscription endpoint is implemented through `/v1/chat/completions`; the existing response bridges preserve the inbound Responses or Anthropic semantics downstream.
- Decision: persist a random account-level `device_id`
  - The same identity is sent during device authorization, refresh, inference, and token counting. Device name/model are stable server descriptors.
- Decision: send the official KimiCLI client identity required by Kimi Coding
  - Device authorization, refresh, and inference consistently send `KimiCLI/1.10.6`, `X-Msh-Platform: kimi_cli`, and the stored account device identity; CPA-specific branding is never sent.

## Risks / Trade-offs

- Kimi may enforce undocumented client-header details
  - Lock the required KimiCLI identity with tests and perform a real-account compatibility probe before release; update the version deliberately when the official client contract changes.
- Device sessions contain sensitive device codes and token results
  - Store only short-lived sessions, redact logs, consume successful tickets once, and never expose refresh tokens through status APIs.
- Kimi tool-call validation is stricter than many OpenAI-compatible upstreams
  - Normalize tool message links and reasoning content only on the Kimi path, with request-body regression tests.
- The current worktree contains overlapping uncommitted account and gateway changes
  - Apply isolated additions and small guarded edits; do not regenerate or reset unrelated files.

## Migration Plan

1. Add the Kimi platform, OAuth client, session store, and management endpoints
2. Register Kimi token refresh and account scheduling
3. Add Kimi gateway routing, headers, model normalization, and protocol compatibility
4. Add the administrator device authorization UI
5. Run focused tests, full service tests, frontend type checking, and strict OpenSpec validation

## Open Questions

- Kimi subscription quota discovery remains a separate capability pending a verified upstream API.
